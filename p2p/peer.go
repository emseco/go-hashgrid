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

package p2p

import (
	"sync"
	"time"

	"go-hashgrid/core"
	"go-hashgrid/log"
)

const (
	baseProtocolLength     = uint64(16)
	baseProtocolMaxMsgSize = 2 * 1024

	pingInterval = 15 * time.Second
)
const (
	// devp2p message codes
	handshakeMsg = 0x00
	discMsg      = 0x01
	pingMsg      = 0x02
	pongMsg      = 0x03
	reqsMsg      = 0x04
	respMsg      = 0x05
	eventMsg     = 0x10
)

const (
	// devp2p message types
	notUsed = 0x00
	normal  = core.Normal
	witness = core.Witness
)

// Peer represents a connected remote node.
type Peer struct {
	rw     *conn
	closed chan struct{}
	wg     sync.WaitGroup
	jp     *core.JPGraph
}

func newPeer(jp *core.JPGraph, conn *conn) *Peer {
	p := &Peer{
		rw:     conn,
		closed: make(chan struct{}),
		jp:     jp,
	}
	return p
}

// ID returns the node's ID.
func (p *Peer) ID() core.Address {
	return p.rw.id
}

func (p *Peer) run() error {
	var (
		err error
		//writeStart = make(chan struct{}, 1)
		//writeErr   = make(chan error, 1)
		readErr = make(chan error, 1)
	)
	p.wg.Add(2)
	go p.readLoop(readErr)
	go p.pingLoop()

	// Start all protocol handlers.
	//writeStart <- struct{}{}
	//p.startProtocols(writeStart, writeErr)

	// Wait for an error or disconnect.
loop:
	for {
		select {
		// case err = <-writeErr:
		// 	if err != nil {
		// 		break loop
		// 	}
		// 	writeStart <- struct{}{}
		case err = <-readErr:
			log.Error("Read error", "error", err)
			if err.Error() != "EOF" {
				break loop
			}
		}
	}

	close(p.closed)
	p.rw.close()
	p.wg.Wait()
	return err
}

func (p *Peer) pingLoop() {
	ping := time.NewTimer(pingInterval)
	defer p.wg.Done()
	defer ping.Stop()
	log.Debug("in peer.pingLoop")
	for {
		select {
		case <-ping.C:
			Send(p.rw, pingMsg, 0, p.ID())
			ping.Reset(pingInterval)
		case <-p.closed:
			return
		}
	}
}

func (p *Peer) readLoop(errc chan<- error) {
	defer p.wg.Done()
	log.Debug("in peer.readLoop")
	for {
		msg, err := p.rw.ReadMsg()
		if err != nil {
			errc <- err
			return
		}
		msg.ReceivedAt = time.Now()
		if err = p.handle(msg); err != nil {
			errc <- err
			return
		}
	}
}

func (p *Peer) handle(msg Msg) error {
	switch {
	case msg.Code == pingMsg:
		go Send(p.rw, pongMsg, 0, p.jp.Coinbase)
		var id core.Address
		msg.Decode(&id)
		// to do ... update active time
	case msg.Code == pongMsg:
		var id core.Address
		msg.Decode(&id)
		// to do ... update active time
	case msg.Code == discMsg:
		// to do ...
		return nil
	case msg.Code == eventMsg:
		var e core.Event
		msg.Decode(&e)
		ch := p.jp.ChanEvent(msg.Type)
		if ch == nil {
			log.Warn("msg type error", "type", msg.Type)
			msg.Discard()
			return nil
		}
		//log.Debug("start to write event ...")
		ch <- &e
		//log.Debug("write event done")
	case msg.Code == reqsMsg:
		var h core.Hash
		var eventType uint32

		msg.Decode(&h)
		event, eventType, err := p.jp.RetrieveEvent(h)
		if err != nil {
			log.Debug("RetriveEvent failed")
			return err
		}
		go Send(p.rw, eventMsg, eventType, event)
	case msg.Code == respMsg:
		// nothing to do ...
	default:
		return msg.Discard()
	}
	return nil
}
