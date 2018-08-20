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
	"errors"
	"math/big"
	"sync"
	"time"

	"go-hashgrid/log"
)

const (
	Normal        = 1
	Witness       = 2
	WitnessLocal  = 3
	FamousWitness = 4

	// Maxium
	MaxArrayLength  = 128
	MaxNodeCount    = 128
	MaxChanItem     = 128
	MaxColorCluster = 4
)

type Vertex struct {
	*Event
	color     [MaxColorCluster]int // color for dfs
	witness   int
	anotherPt *Vertex
	selfPt    *Vertex
	seqNum    int
}

type Column struct {
	hashChain map[Hash]*Vertex
	verTop    *Vertex // Vertex on the top
	lock      *sync.RWMutex
	seqNum    int
}

type JPGraph struct {
	graph         map[Address]*Column
	superMajority int

	witnessL  chan *Event // local
	witnessR  chan *Event // remote
	eventNorm chan *Event // normal event

	transUnHandled []*Transaction
	rManager       *RoundManager
	transLock      *sync.RWMutex
	graphLock      *sync.RWMutex

	// public members
	Coinbase         Address
	broadness        int
	RequestChan      chan *RequestArg
	SelfEventCreated chan *Vertex
	roundTrigger     chan *Vertex // trigger the round
}

func (jp *JPGraph) TransUnHandled() []*Transaction {
	return jp.transUnHandled
}

func (jp *JPGraph) ChanEvent(tp uint32) chan *Event {
	switch tp {
	case Normal:
		return jp.eventNorm
	case Witness:
		return jp.witnessR
	case WitnessLocal:
		return jp.witnessL
	default:
		return nil
	}

	return nil
}

func NewColumn() *Column {
	c := new(Column)
	if c != nil {
		c.hashChain = make(map[Hash]*Vertex)
		c.verTop = nil
		c.lock = new(sync.RWMutex)
	}

	return c
}

func NewJPGraph(coinbase Address, broadness int) *JPGraph {
	jp := new(JPGraph)
	if jp != nil {
		jp.graph = make(map[Address]*Column)

		jp.witnessL = make(chan *Event, MaxChanItem)
		jp.witnessR = make(chan *Event, MaxChanItem)
		jp.eventNorm = make(chan *Event, MaxChanItem)
		jp.roundTrigger = make(chan *Vertex, MaxChanItem)
		jp.SelfEventCreated = make(chan *Vertex, MaxChanItem)
		jp.RequestChan = make(chan *RequestArg, MaxChanItem)

		jp.Coinbase = coinbase
		jp.broadness = broadness

		log.Debug("NewJPGraph", "jp.Coinbase", jp.Coinbase)

		jp.transLock = new(sync.RWMutex)
		jp.graphLock = new(sync.RWMutex)

		jp.AddNode(jp.Coinbase)
		jp.rManager = NewRoundManager(jp)
	}

	return jp
}

func NewVertex(e *Event, ty int) *Vertex {
	v := &Vertex{
		witness:   ty,
		anotherPt: nil,
		selfPt:    nil,
	}
	v.Event = CopyEvent(e)

	return v
}

func (jp *JPGraph) Signature(s *Sig) {
	s.Addr = jp.Coinbase // array assignment means copy
}

// AddEvent convert an event as a Vertex, add to jp.graph
func (jp *JPGraph) AddEvent(e *Event, ty int) error {
	// if existed, return
	var linkpost bool = true
	var firstWitness bool = false

	// validate the Event
	if jp.validateEvent(e) == false {
		log.Debug("Addevent: validate Event failed")
		return errors.New("Event Validate check failed")
	}

	var otherColumn *Column
	var ok2 bool = true
	jp.graphLock.RLock()
	selfColumn, ok1 := jp.graph[e.SelfSig.Addr]
	if e.OtherPartA.Compare(&Address{0}) != 0 {
		otherColumn, ok2 = jp.graph[e.OtherPartA]
	}
	jp.graphLock.RUnlock()

	if !ok1 {
		selfColumn = jp.AddNode(e.SelfSig.Addr)
		firstWitness = true
	}
	if !ok2 {
		otherColumn = jp.AddNode(e.OtherPartA)
	}

	selfColumn.lock.Lock()
	vert, ok := selfColumn.hashChain[e.SelfHash]
	if ok {
		// already existed
		selfColumn.lock.Unlock()
		if ty == Witness {
			vert.witness = ty
			jp.roundTrigger <- vert
		}
		log.Debug("AddEvent: Event already existed", "Hash", vert.SelfHash)
		return nil
	}

	vert = NewVertex(e, ty) // set the vertex type
	vert.selfPt = selfColumn.verTop
	vert.seqNum = selfColumn.seqNum
	selfColumn.hashChain[vert.SelfHash] = vert

	selfColumn.verTop = vert
	selfColumn.lock.Unlock()

	if otherColumn != nil {
		otherColumn.lock.RLock()
		otherp, ok := otherColumn.hashChain[vert.OtherPartH]
		otherColumn.lock.RUnlock()

		if ok {
			vert.anotherPt = otherp
			linkpost = false
		}
	}

	if linkpost {
		// request an Event
		jp.RequestParentEvent(vert)
	}

	// add the info to round
	l := jp.rManager.queue
	var round *Round
	for e := l.Front(); e != nil; e = e.Next() {
		round, ok = (e.Value).(*Round)
		if vert.RoundID.Cmp(big.NewInt(int64(round.index))) == 0 {
			break
		}
	}

	if round != nil {
		if firstWitness || ty == Witness ||
					len(selfColumn.hashChain) == 1 {
			vert.witness = Witness
			i := selfColumn.seqNum
			round.witnesses[i] = vert
			log.Info("AddEvent: round Witness", "seq", i)
		}

		round.nc.Lock()
		_, ok = round.newComer[vert.SelfHash]
		if !ok {
			round.newComer[vert.SelfHash] = vert.SelfSig.Addr
		}
		round.nc.Unlock()

		round.ld.Lock()
		_, ok = round.lostDict[vert.SelfHash]
		if ok {
			for _, v := range round.lostDict[vert.SelfHash] {
				v.anotherPt = vert
			}
			delete(round.lostDict, vert.SelfHash)
		}

		if linkpost {
			oph := vert.OtherPartH
			if _, ok = round.lostDict[oph]; !ok {
				ar := make([]*Vertex, 0, MaxArrayLength)
				round.lostDict[oph] = ar
			}
			round.lostDict[oph] = append(round.lostDict[oph], vert)
		}
		round.ld.Unlock()
	}

	if vert.SelfSig.Addr.Compare(&jp.Coinbase) == 0 {
		log.Debug("SelfEventCreated")
		if vert.witness == Normal {
			jp.SelfEventCreated <- vert
		}
	}

	return nil
}

//AddNode add node(computer) to JPGraph, it forms a column
func (jp *JPGraph) AddNode(a Address) *Column {
	jp.graphLock.RLock()
	if cl, ok := jp.graph[a]; ok {
		jp.graphLock.RUnlock()
		return cl
	}
	jp.graphLock.RUnlock()

	cl := NewColumn()
	if cl != nil {
		jp.graphLock.Lock()
		mlen := len(jp.graph)
		cl.seqNum = mlen
		jp.graph[a] = cl
		jp.graphLock.Unlock()

		cl.verTop = nil
		jp.superMajority = int((mlen + 1) * 2 / 3)
		log.Info("AddNode New Column:", "mlen", mlen, "Address", a)
	} else {
		log.Warn("AddNode: NewColumn failed")
	}
	return cl
}

//DeleteNode delete a node
func (jp *JPGraph) DeleteNode(a Address) {

}

//CurrentRound return roundID in roundManager
func (jp *JPGraph) CurrentRound() int { return jp.rManager.currentIdx }

func (jp *JPGraph) RequestParentEvent(vert *Vertex) {
	req := &RequestArg{}
	req.TargetHash = vert.OtherPartH
	req.TargetAddr = vert.SelfSig.Addr

	jp.RequestChan <- req
}

//ComposeEvent compose an Event
func (jp *JPGraph) ComposeEvent() *Event {
	var et *Event

	et = new(Event)
	et.TimeStamp = big.NewInt(time.Now().UnixNano())
	et.RoundID = big.NewInt(int64(jp.CurrentRound()))
	mlen := len(jp.transUnHandled)
	if mlen > MaxTransInEvent {
		mlen = MaxTransInEvent
	}
	if mlen > 0 {
		et.Trans = make([]*Transaction, 0, MaxTransInEvent)
		copy(et.Trans, jp.transUnHandled[:mlen])
		jp.transUnHandled = jp.transUnHandled[mlen+1:]
	}

	jp.graphLock.RLock()
	col := jp.graph[jp.Coinbase]
	jp.graphLock.RUnlock()

	if col.verTop != nil {
		et.SelfPart = col.verTop.SelfHash
		et.RoundID = col.verTop.RoundID
	}

	jp.Signature(&et.SelfSig)

	element := jp.rManager.queue.Front()
	round, ok := (element.Value).(*Round)
	if ok {
		if round.bestVertex != nil {
			et.OtherPartH = round.bestVertex.SelfHash
			et.OtherPartA = round.bestVertex.SelfSig.Addr
		} else {
			// select a rand one from round.newComer
			arg := round.RandParent()
			if arg != nil {
				et.OtherPartH = arg.TargetHash
				et.OtherPartA = arg.TargetAddr
			}
		}
	}

	et.SelfHash = et.Hash()
	et.RoundID.SetInt64(int64(round.index))
	log.Debug("ComposeEvent", "Hash", et.SelfHash, "Round", round.index)

	return et
}

// RetrieveEvent get an event from self column
func (jp *JPGraph) RetrieveEvent(hash Hash) (*Event, uint32, error) {
	var eventType uint32

	jp.graphLock.RLock()
	column, ok := jp.graph[jp.Coinbase]
	if !ok {
		jp.graphLock.RUnlock()
		return nil, eventType, errors.New("RetrieveEvent: coinbase not in graph")
	}
	jp.graphLock.RUnlock()

	column.lock.RLock()
	defer column.lock.RUnlock()

	for _, v := range column.hashChain {
		if v.OtherPartH == hash && v.anotherPt != nil {
			e := v.anotherPt.Event
			eventType = (uint32)(v.witness)
			return e, eventType, nil
		}
	}

	return nil, eventType, errors.New("RetrieveEvent: not found")
}

func (jp *JPGraph) validateEvent(e *Event) bool {
	// Hash validate
	nEvent := CopyEvent(e)
	nEvent.SelfHash = Hash{0}

	var nHash = nEvent.Hash()
	if nHash.Compare(&e.SelfHash) == 0 {
		return true
	}

	return false
	// Transaction and Signature validate
}
