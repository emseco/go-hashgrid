/**
* Copyright (c) 2018 EMSECO 
* This source code is being disclosed to you solely for the purpose of your participation in 
* testing EMSECO. You may view, compile and run the code for that purpose and pursuant to 
* the protocols and algorithms that are programmed into, and intended by, the code. You may 
* not do anything else with the code without express permission from EMSECO Foundation Ltd., 
* including modifying or publishing the code (or any part of it), and developing or forming 
* another public or private blockchain network. This source code is provided ‘as is’ and no 
* warranties are given as to title or non-infringement, merchantability or fitness for purpose 
* and, to the extent permitted by law, all liability for your use of the code is disclaimed. 
* Some programs in this code are governed by the GNU General Public License v3.0 (available at 
* https://www.gnu.org/licenses/gpl-3.0.en.html) (‘GPLv3’). The programs that are governed by 
* GPLv3.0 are those programs that are located in the folders src/depends and tests/depends 
* and which include a reference to GPLv3 in their program files.
**/

package core

import (
	"container/list"
	"go-hashgrid/log"
	"math/big"
	"math/rand"
	"sort"
	"sync"
)

type RoundManager struct {
	queue      *list.List
	jp         *JPGraph
	currentIdx int

	// use for moniotr
	monitorStarter *Vertex

	witTrigger       chan int
	lostTrigger      chan int
	consensusTrigger chan int
}

const (
	ColorClusterRound = iota
	ColorClusterVote
	ColorClusterConsu
	RoundMax = 10
)

type Round struct {
	ld sync.RWMutex
	nc sync.RWMutex

	lostDict   map[Hash][]*Vertex
	newComer   []*RequestArg
	bestVertex *Vertex
	witnesses  []*Vertex
	broadness  int
	index      int

	votes     [][]int
	strongsee [][]bool
}

func NewRoundManager(jp *JPGraph) *RoundManager {
	rdm := &RoundManager{}
	rdm.jp = jp
	rdm.queue = list.New()

	rd := NewRound(jp.broadness, 0)
	rdm.queue.PushFront(rd)
	rdm.witTrigger = make(chan int, MaxChanItem)
	rdm.lostTrigger = make(chan int, MaxChanItem)
	rdm.consensusTrigger = make(chan int, MaxChanItem)

	// handle the roop of JPGraph
	go rdm.RoundLoop()
	go rdm.MonitorLoop()
	go rdm.ConsensusShow()

	return rdm
}

//RoundLoop handles the round in jigsaw-puzzle
func (rdm *RoundManager) RoundLoop() {
	var wtriggered bool
	var vert *Vertex
	var round *Round

	for {
		select {
		case vert = <-rdm.jp.roundTrigger:
			// trigger when one witness comes (local or remote)
			log.Info("RoundLoop in roundTrigger", "rdm.currentIdx", rdm.currentIdx, "Addr", vert.SelfSig.Addr)

			if (vert.witness == WitnessLocal && vert.RoundID.Cmp(big.NewInt(int64(rdm.currentIdx))) == 0) ||
				(vert.witness == Witness && vert.RoundID.Cmp(big.NewInt(int64(rdm.currentIdx+1))) == 0) {
				rdm.currentIdx++
				vert.RoundID.SetInt64(int64(rdm.currentIdx))
				rd := NewRound(rdm.jp.broadness, rdm.currentIdx)
				rdm.queue.PushFront(rd)
				wtriggered = false
			}
			// sometime, vert.RoundID may more bigger than rdm.currentIdx

			for e := rdm.queue.Front(); e != nil; e = e.Next() {
				round, _ = (e.Value).(*Round)
				if vert.RoundID.Cmp(big.NewInt(int64(round.index))) == 0 {
					break
				}
				round = nil
			}

			if round != nil {
				idx := vert.seqNum
				round.witnesses[idx] = vert
				log.Info("RoundLoop :set witness", "roundID", round.index, "seqNum", idx)

				for idx = 0; idx < round.broadness; idx++ {
					if round.witnesses[idx] == nil {
						// this witness hasn't come
						break
					}
				}
				if idx == round.broadness {
					rdm.consensusTrigger <- round.index
					log.Debug("RoundLoop: consensus Trigger", "round", round.index)
				}

				// preElement := rdm.queue.Front().Next()
				// if preElement != nil {
				// 	p1Round, ok := (preElement.Value).(*Round)
				// 	if ok {
				// 		if len(p1Round.lostDict) == 0 {
				// 			go rdm.WitnessFame()
				// 		} else {
				// 			wtriggered = true
				// 		}
				// 	}
				// }
			}

		case <-rdm.lostTrigger:
			log.Debug("rdm.lostTrigger")
			if wtriggered {
				go rdm.WitnessFame()
			}
		}
	}
}

//MonitorLoop Monitor the graph, want to find witness
func (rdm *RoundManager) MonitorLoop() {
	roundNum := 0
	for {
		Element := rdm.queue.Front()
		round := (Element.Value).(*Round)

		select {
		case e := <-rdm.jp.SelfEventCreated:
			rdm.monitorStarter = e
		}

		mlen := len(rdm.jp.graph)
		if rdm.monitorStarter == nil || rdm.jp.broadness != mlen {
			log.Debug("MonitorLoop: in Sleep", "graph Length", mlen)
			continue
		}

		log.Debug("MonitorLoop Search for witness", "round", round.index)
		idx := round.index
		strongsee := 0
		roundNum++

		for i := 0; i < round.broadness; i++ {
			m := round.witnesses[i]
			if m != nil {
				rdm.clearColorDFS(idx, ColorClusterRound)
				_, k := rdm.JudgeSee(rdm.monitorStarter, m, ColorClusterRound)
				if k {
					strongsee++
				}
			}
		}

		if strongsee > rdm.jp.superMajority || (rdm.monitorStarter.seqNum == 0 && roundNum == RoundMax) {
			// find the witness local
			log.Debug("Find local witness", "round", round.index)

			rdm.jp.witnessL <- rdm.monitorStarter.Event
			rdm.monitorStarter.witness = WitnessLocal
			rdm.jp.roundTrigger <- rdm.monitorStarter

			roundNum = 0
		}
	}
}

// WitnessFame calc and get famous witness
func (rdm *RoundManager) WitnessFame() {
	var p1Round *Round
	var p2Round *Round
	var ok1, ok2 bool
	var totalVotes []int

	log.Debug("WitnessFame starts")
	Element := rdm.queue.Front()
	round := (Element.Value).(*Round)
	preElement := Element.Next()
	if preElement != nil {
		p1Round, ok1 = (preElement.Value).(*Round)
		if ok1 {
			mpElement := preElement.Next()
			if mpElement != nil {
				p2Round, ok2 = (mpElement.Value).(*Round)
			}
		}
	}

	if ok1 {
		for i, n := range round.witnesses {
			for j, m := range p1Round.witnesses {
				rdm.clearColorDFS(p1Round.index, ColorClusterVote)
				round.votes[i][j], round.strongsee[i][j] = rdm.JudgeSee(n, m, ColorClusterVote)
			}
		}

		// collect votes
		totalVotes = make([]int, round.broadness)
		for i := 0; i < round.broadness; i++ {
			v := 0
			for j, _ := range round.votes {
				if round.strongsee[i][j] == true {
					// strong see, collect vote
					v += round.votes[i][j]
				}
			}
			totalVotes[i] = v
		}
	}

	if ok2 {
		for i, v := range totalVotes {
			if v > rdm.jp.superMajority {
				// ok, famous witness born
				p2Round.witnesses[i].witness = FamousWitness
			}
		}
	}

	return
}

type AddrSlice []Address

func (s AddrSlice) Len() int      { return len(s) }
func (s AddrSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s AddrSlice) Less(i, j int) bool {
	var a, b int
	for x := 0; x < AddressLength; x++ {
		a += int(s[i][x])
		b += int(s[j][x])
	}
	return a < b
}

//ConsensusShow display the blocks
func (rdm *RoundManager) ConsensusShow() {
	var addrPre []Address
	var totalBlocks int64

	for {
		select {
		case idx := <-rdm.consensusTrigger:
			log.Info("Consensus Reached:", "round", idx)
			addrPre = make([]Address, 0, MaxArrayLength)
			rdm.jp.graphLock.RLock()
			for addr, _ := range rdm.jp.graph {
				addrPre = append(addrPre, addr)
			}
			rdm.jp.graphLock.RUnlock()
			sort.Sort(AddrSlice(addrPre))

			for _, addr := range addrPre {
				log.Info("Consensus", "Addr", addr)
				rdm.jp.graphLock.RLock()
				col, ok := rdm.jp.graph[addr]
				rdm.jp.graphLock.RUnlock()

				if ok {
					col.lock.RLock()
					x := col.verTop
					if x != nil && x.witness == Witness {
						log.Info("Consensus Block", "Hash", x.SelfHash)
						totalBlocks++
						x = x.selfPt
					}
					for ; x != nil && x.witness != Witness; x = x.selfPt {
						log.Info("Consensus Block", "Hash", x.SelfHash)
						totalBlocks++
					}
					col.lock.RUnlock()
				}
			}
			log.Info("Consensus ...", "totalBlocks", totalBlocks)
		}
	}
}

// NewRound construct a round
func NewRound(l, idx int) *Round {
	rd := new(Round)
	rd.lostDict = make(map[Hash][]*Vertex)
	rd.newComer = make([]*RequestArg, 0, MaxArrayLength)
	rd.broadness = l
	rd.index = idx

	for i := 0; i < rd.broadness; i++ {
		tmp := make([]int, rd.broadness)
		tmp2 := make([]bool, rd.broadness)
		rd.votes = append(rd.votes, tmp)
		rd.strongsee = append(rd.strongsee, tmp2)
	}
	rd.witnesses = make([]*Vertex, MaxNodeCount)

	return rd
}

//
func (rdm *RoundManager) clearColorDFS(idx int, colorCluster int) {
	var starter *Vertex

	rdm.jp.graphLock.RLock()
	defer rdm.jp.graphLock.RUnlock()

	for _, col := range rdm.jp.graph {
		col.lock.RLock()
		for s := col.verTop; s != nil; s = col.hashChain[s.SelfPart] {
			if s.RoundID.Cmp(big.NewInt(int64(idx))) == 0 {
				starter = s
				break
			}
		}
		for s := starter; s != nil && s.RoundID.Cmp(big.NewInt(int64(idx))) == 0; s = col.hashChain[s.SelfPart] {
			s.color[colorCluster] = WHITE
		}
		col.lock.RUnlock()
	}
	return
}

// JudgeSee decide the 2 witness see/strong see
func (rdm *RoundManager) JudgeSee(s *Vertex, e *Vertex, colorCluster int) (int, bool) {
	var trace []int
	total := 0
	see := 0
	strongsee := false

	trace = make([]int, rdm.jp.broadness)
	if s == nil || e == nil {
		log.Debug("JudgeSee: start or end is nil")
		return 0, false
	}
	rdm.dfsVisit(trace, s, e, colorCluster) // trace is
	index := e.seqNum

	if trace[index] == 1 {
		see = 1
	}
	for _, t := range trace {
		total += t
	}
	if total >= rdm.jp.superMajority {
		strongsee = true
	}
	return see, strongsee
}

const (
	WHITE = 0
	GREY  = 1
	BLACK = 2
)

// dfsVisit deep first search
func (rdm *RoundManager) dfsVisit(trace []int, s *Vertex, e *Vertex, colorCluster int) {
	var x [2]*Vertex

	if s == nil {
		return
	}

	index := s.seqNum
	if s == e || s.witness == Witness {
		trace[index] = 1
		return
	}

	s.color[colorCluster] = GREY
	trace[index] = 1

	x[0] = s.selfPt
	x[1] = s.anotherPt
	for _, u := range x {
		if u != nil && u.color[colorCluster] == WHITE {
			rdm.dfsVisit(trace, u, e, colorCluster)
		}
	}
	s.color[colorCluster] = BLACK
}

func (rd *Round)addComer(rqa *RequestArg) {
	rd.nc.Lock()
	for _, n := range rd.newComer {
		if n.TargetHash.Compare(&rqa.TargetHash) == 0 {
			// already in rd.newComer
			return
		}
	}

	rd.newComer = append(rd.newComer, rqa)
	rd.nc.Unlock()

	return
}

// RandParent retrieve a parent from newComer randomly
func (rd *Round) RandParent(except Address) (arg *RequestArg) {
	ret := &RequestArg{}

	rd.nc.RLock()
	mlen := len(rd.newComer)
	if mlen == 0 {
		rd.nc.RUnlock()
		return nil
	}
	to := rand.Intn(mlen)

	for i, arg := range rd.newComer {
		if arg.TargetAddr.Compare(&except) != 0 {
			ret = arg			
		}

		if i == to {
			if arg.TargetAddr.Compare(&except) != 0 {
				ret = arg
				arg.Refered++
				break
			}
		}
	}
	rd.nc.RUnlock()
	if ret.TargetAddr.Compare(&Address{}) == 0 {
		return ret		
	}
	return nil
}