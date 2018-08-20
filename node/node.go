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

package node

import (
	"math/big"
	"time"

	"go-hashgrid/core"
	"go-hashgrid/log"
	"go-hashgrid/p2p"
)

type Node struct {
	jigsaw   *core.JPGraph
	server   *p2p.Server
	interval int
}

func (nd *Node) NodeConfig() {

}

func NewNode(gc *core.GlobalConfig) *Node {
	node := &Node{}
	broadness := len(gc.Peers)
	node.jigsaw = core.NewJPGraph(gc.Coinbase, broadness)

	log.Info("NewNode", "InterMilli", gc.InterMilli)
	node.interval = gc.InterMilli

	// Start the p2p
	node.server = &p2p.Server{}
	node.server.Start(node.jigsaw, gc)
	return node
}

// EventHandle receive the event form graph/MsgHandler, add the event
func (nd *Node) EventHandle() {
	var et *core.Event
	var req *core.RequestArg
	var ty int
	var roundComming bool

	jp := nd.jigsaw

	myInterval := 1000
	if nd.interval >= 100 && nd.interval <= 1000 {
		myInterval = nd.interval
	}
	onlyTime := time.NewTimer(time.Microsecond * time.Duration(myInterval))
	overFlow := time.NewTimer(time.Microsecond * core.OverflowDuration)

	eventNorm := jp.ChanEvent(core.Normal)
	witnessR := jp.ChanEvent(core.Witness)
	witnessL := jp.ChanEvent(core.WitnessLocal)

	transUnHandled := jp.TransUnHandled()

	for {
		select {
		case et = <-eventNorm:
			log.Debug("Receive Event")
			ty = core.Normal

		case et = <-witnessL:
			// receive the local witness, if local, broadcast it
			ty = core.WitnessLocal
			go nd.server.BroadcastEvent(et, uint32(core.Witness))

		case et = <-witnessR:
			// receive the remote witness
			ty = core.Witness
			log.Debug("EventHandle, witnessR")
			if et.RoundID.Cmp(big.NewInt(int64(jp.CurrentRound()+1))) == 0 {
				roundComming = true
			}

		case <-onlyTime.C:
			//log.Info("onlyTime hit")
			et = jp.ComposeEvent()
			var etTy uint32
			if roundComming == false {
				etTy = core.Normal
			} else {
				etTy = core.Witness
			}
			log.Debug("onlyTime.C", "etTy", etTy)
			go nd.server.BroadcastEvent(et, etTy)

			roundComming = false
			onlyTime = time.NewTimer(time.Millisecond * core.EventDuration)

		case <-overFlow.C:
			// check the trans unhandled and Hash received
			if len(transUnHandled) > core.MaxTransInEvent {
				et = jp.ComposeEvent()

				// var etTy uint32
				// if roundComming == false {
				// 	etTy = core.Normal
				// } else {
				// 	etTy = core.Witness
				// }

				roundComming = false
			}
			overFlow = time.NewTimer(time.Millisecond * core.OverflowDuration)

		case req = <-jp.RequestChan:
			go nd.server.RequestEvent(req.TargetAddr, req.TargetHash)
		}

		// add new event to graph, witnessLocal has been Added!
		if ty != core.WitnessLocal && et != nil {
			go jp.AddEvent(et, ty)
		}
	}
}
