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
	"bytes"
	"errors"
	"go-hashgrid/core"
	"go-hashgrid/log"
	"io"
	"math/big"
	"math/rand"
	"net"
	"sync"
)

// Server manages all peer connections.
type Server struct {
	lock         sync.Mutex // protects running
	running      bool
	jp           *core.JPGraph
	id           core.Address
	listener     net.Listener
	ListenAddr   string
	log          log.Logger
	peerOp       chan peerOpFunc // These are for Peers, PeerCount (and nothing else).
	peerOpDone   chan struct{}
	addpeer      chan *conn
	delpeer      chan peerDrop
	addstatic    chan *Node
	removestatic chan *Node
	quit         chan struct{}
	loopWG       sync.WaitGroup // loop, listenLoop
}

type peerOpFunc func(map[core.Address]*Peer)

// conn wraps a network connection with information gathered
// during the two handshakes.
type conn struct {
	fd   net.Conn
	id   core.Address
	name string
	MsgReadWriter
}

func (c *conn) close() {
	c.fd.Close()
}

func (c *conn) ReadMsg() (msg Msg, err error) {
	if c.fd == nil {
		return msg, errors.New("ReadMsg error: connection not exist")
	}

	// read the header
	headbuf := make([]byte, 9)
	if _, err := io.ReadFull(c.fd, headbuf); err != nil {
		return msg, err
	}

	log.Debug("recv raw", "head", headbuf)
	msg.Code = headbuf[0]
	msg.Type = readInt32(headbuf[1:])
	msg.Size = readInt32(headbuf[5:])
	log.Debug("Recv Msg from peer", "code", msg.Code, "type", msg.Type, "id", c.id)

	// read the payload
	framebuf := make([]byte, msg.Size)
	if _, err := io.ReadFull(c.fd, framebuf); err != nil {
		return msg, err
	}
	log.Debug("recv raw", "data", framebuf)

	content := bytes.NewReader(framebuf[:msg.Size])
	msg.Payload = content
	return msg, nil
}

func (c *conn) WriteMsg(msg Msg) error {
	if c.fd == nil {
		return errors.New("WriteMsg error: connection not exist")
	}

	log.Debug("Send Msg to peer", "code", msg.Code, "type", msg.Type, "id", c.id)

	// write header
	buff := make([]byte, 9+msg.Size)
	buff[0] = msg.Code
	putInt32(msg.Type, buff[1:])
	putInt32(msg.Size, buff[5:])

	// write payload
	n, err := msg.Payload.Read(buff[9:])
	if n != int(msg.Size) {
		return errors.New("WriteMsg error: payload size not match")
	}
	log.Debug("send raw", "data", buff)
	_, err = c.fd.Write(buff)
	return err
}

func readInt32(b []byte) uint32 {
	return uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
}

func putInt32(v uint32, b []byte) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 26)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

type peerDrop struct {
	*Peer
	err error
}

// NodeIDBits ...
const NodeIDBits = 512

// Node represents a host on the network.
// The fields of Node may not be modified.
type Node struct {
	addr string
	ID   core.Address // the node's public key
}

// FindPeer find the peer by coinbase
func (srv *Server) FindPeer(coinbase core.Address) *Peer {
	var p *Peer
	select {
	case srv.peerOp <- func(peers map[core.Address]*Peer) {
		p = peers[core.Address(coinbase)]
	}:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return p
}

// Peers returns all connected peers.
func (srv *Server) Peers() []*Peer {
	var ps []*Peer
	select {
	// Note: We'd love to put this function into a variable but
	// that seems to cause a weird compiler error in some
	// environments.
	case srv.peerOp <- func(peers map[core.Address]*Peer) {
		for _, p := range peers {
			ps = append(ps, p)
		}
	}:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return ps
}

// PeerCount returns the number of connected peers.
func (srv *Server) PeerCount() int {
	var count int
	select {
	case srv.peerOp <- func(ps map[core.Address]*Peer) { count = len(ps) }:
		<-srv.peerOpDone
	case <-srv.quit:
	}
	return count
}

// AddPeer connects to the given node and maintains the connection until the
// server is shut down. If the connection fails for any reason, the server will
// attempt to reconnect the peer.
func (srv *Server) AddPeer(node *Node) {
	select {
	case srv.addstatic <- node:
	case <-srv.quit:
	}
}

// RemovePeer disconnects from the given node
func (srv *Server) RemovePeer(node *Node) {
	select {
	case srv.removestatic <- node:
	case <-srv.quit:
	}
}

// Stop terminates the server and all active peer connections.
// It blocks until all active connections have been closed.
func (srv *Server) Stop() {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if !srv.running {
		return
	}
	srv.running = false
	if srv.listener != nil {
		srv.listener.Close()
	}
	close(srv.quit)
	srv.loopWG.Wait()
}

// Start starts running the server.
// Servers can not be re-used after stopping.
func (srv *Server) Start(jp *core.JPGraph, config *core.GlobalConfig) (err error) {
	srv.lock.Lock()
	defer srv.lock.Unlock()
	if srv.running {
		return errors.New("server already running")
	}
	log.Info("Starting P2P networking")

	srv.quit = make(chan struct{})
	srv.addpeer = make(chan *conn)
	srv.delpeer = make(chan peerDrop)
	srv.peerOp = make(chan peerOpFunc)
	srv.peerOpDone = make(chan struct{})
	srv.addstatic = make(chan *Node)
	srv.removestatic = make(chan *Node)
	srv.ListenAddr = config.Addr
	srv.id = config.Coinbase
	srv.jp = jp

	// listen/dial
	if srv.ListenAddr != "" {
		if err := srv.startListening(); err != nil {
			return err
		}
	}

	srv.loopWG.Add(1)
	go srv.run()

	srv.running = true

	// static add peers
	for _, p := range config.Peers {
		n := &Node{addr: p}
		srv.addstatic <- n
	}
	return nil
}

func (srv *Server) startListening() error {
	// Launch the TCP listener.
	listener, err := net.Listen("tcp", srv.ListenAddr)
	if err != nil {
		return err
	}
	laddr := listener.Addr().(*net.TCPAddr)
	srv.ListenAddr = laddr.String()
	srv.listener = listener
	srv.loopWG.Add(1)
	go srv.listenLoop()

	return nil
}

func (srv *Server) run() {
	defer srv.loopWG.Done()
	var peers = make(map[core.Address]*Peer)
running:
	for {
		select {
		case <-srv.quit:
			break running
		case n := <-srv.addstatic:
			// This channel is used by AddPeer to add to the
			// ephemeral static peer list. Add it to the dialer,
			// it will keep the node connected.
			log.Info("Adding static node", "node", n)
			fd, err := net.Dial("tcp", n.addr)
			if err != nil {
				log.Error("Adding static node failed", "error", err)
				continue
			}
			go srv.HandleConn(fd)
		case n := <-srv.removestatic:
			// This channel is used by RemovePeer to send a
			// disconnect request to a peer and begin the
			// stop keeping the node connected
			log.Info("Removing static node", "node", n)
			// to do ...
		case op := <-srv.peerOp:
			// This channel is used by Peers and PeerCount.
			op(peers)
			srv.peerOpDone <- struct{}{}
		case c := <-srv.addpeer:
			if peers[c.id] != nil {
				log.Warn("Adding p2p peer already exist", "peer", c.id)
				break
			}
			p := newPeer(srv.jp, c)
			go srv.runPeer(p)
			peers[c.id] = p
			log.Info("Adding p2p peer", "peer", c.id, "count", len(peers))
		case pd := <-srv.delpeer:
			log.Info("Removing p2p peer")
			delete(peers, pd.ID())
		}
	}
}

// listenLoop runs in its own goroutine and accepts inbound connections.
func (srv *Server) listenLoop() {
	defer srv.loopWG.Done()
	for {
		fd, err := srv.listener.Accept()
		if err != nil {
			log.Error("Accept error")
			return
		}

		log.Info("Accepted connection", "peer", fd.RemoteAddr())
		go srv.HandleConn(fd)
	}
}

// HandleConn runs the handshakes and attempts to add the connection as a peer.
func (srv *Server) HandleConn(fd net.Conn) error {
	c := &conn{fd: fd}
	recvID, err := srv.protoHandshake(c)
	if err != nil {
		c.close()
		log.Warn(err.Error())
		return err
	}
	if recvID.Compare(&srv.id) == 0 {
		c.close()
		log.Warn("remote peer has the same ID")
		return nil
	}

	// check peer
	if p := srv.FindPeer(recvID); p != nil {
		fd.Close()
		return nil
	}

	c.id = recvID
	select {
	case srv.addpeer <- c:
	case <-srv.quit:
		log.Warn("Rejected peer")
		return nil
	}

	return nil
}

func (srv *Server) protoHandshake(rw *conn) (core.Address, error) {
	log.Debug("send handshake msg", "id", srv.id)
	Send(rw, handshakeMsg, 0, srv.id)
	msg, err := rw.ReadMsg()
	if err != nil {
		return core.Address(srv.id), err
	}
	var recvID core.Address
	msg.Decode(&recvID)
	log.Debug("receive handshake msg", "id", recvID)
	if recvID.Compare(&srv.id) == 0 {
		return recvID, errors.New("remote peer has the same ID")
	}
	return recvID, nil
}

// runPeer runs in its own goroutine for each peer.
// it waits until the Peer logic returns and removes
// the peer.
func (srv *Server) runPeer(p *Peer) {
	err := p.run()
	srv.delpeer <- peerDrop{p, err}
}

// GossipEvent use Gossip protocol send Event to neighbor(s)
func (srv *Server) GossipEvent(e *core.Event, eventType uint32) error {
	peers := srv.Peers()
	if peers == nil || len(peers) == 0 {
		return errors.New("Can not find any peers")
	}
	p := peers[rand.Intn(len(peers))]
	return Send(p.rw, eventMsg, eventType, *e)
}

// BroadcastEvent broadcast event to all neighbors
func (srv *Server) BroadcastEvent(e *core.Event, eventType uint32) error {
	peers := srv.Peers()
	if peers == nil || len(peers) == 0 {
		return errors.New("Can not find any peers")
	}
	for _, p := range peers {
		err := Send(p.rw, eventMsg, eventType, *e)
		if err != nil {
			log.Error("BroadcastEvent Send error", "err", err)
		}
	}
	return nil
}

// RequestEvent request event from the Peer
func (srv *Server) RequestEvent(base core.Address, hash core.Hash) error {
	p := srv.FindPeer(base)
	if p == nil {
		return errors.New("wrong coinbase")
	}
	if hash.Big().Cmp(big.NewInt(0)) == 0 {
		return errors.New("Invalid hash")
	}

	return Send(p.rw, reqsMsg, 0, hash)
}
