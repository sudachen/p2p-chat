package chat

import (
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	statusCode           = 0
	messagesCode         = 1
	NumberOfMessageCodes = 255
)

const ProtocolName = "cht"
const ProtocolVersion = uint64(1) // ATTENTION exactly uit64
const ProtocolVersionStr = "1.0"

const batchLength = 10
const broadcastTimeout = 100 * time.Millisecond

type peer struct {
	board *board
	p2    *p2p.Peer
	rw    p2p.MsgReadWriter
}

func protocols(brd *board, p2pMaxPkgSize uint32) []p2p.Protocol {
	return []p2p.Protocol{
		p2p.Protocol{
			Name:    ProtocolName,
			Version: uint(ProtocolVersion),
			Length:  NumberOfMessageCodes,
			NodeInfo: func() interface{} {
				return map[string]interface{}{
					"version":        ProtocolVersionStr,
					"maxMessageSize": p2pMaxPkgSize,
				}
			},
			Run: func(p2 *p2p.Peer, mrw p2p.MsgReadWriter) error {
				return (&peer{brd, p2, mrw}).loop(p2pMaxPkgSize)
			},
		},
	}
}

func (p *peer) ID() []byte {
	id := p.p2.ID()
	return id[:]
}

func (p *peer) broadcast(quit chan struct{}) {
	t := time.NewTicker(broadcastTimeout)
	var index int64
	batch := make([][]byte, batchLength)
	for {
		if done2(quit, t.C) {
			return
		}

		now := time.Now().Unix()
		n := 0
		m, i := p.board.get(index)
	FillBatch:
		for m != nil {
			if done(quit) {
				return
			}
			if m.deathTime() > now {
				batch[n] = m.body
				n++
			}
			if n == batchLength {
				break FillBatch
			}
			m, i = p.board.get(i)
		}

		if n != 0 {
			if err := p2p.Send(p.rw, messagesCode, batch[:n]); err != nil {
				log.Warn("failed to send messages", "peer", p.ID, "err", err)
			} else {
				index = i
			}
		}
	}
}

func (p *peer) handshake() error {
	ec := make(chan error, 1)
	go func() {
		ec <- p2p.SendItems(p.rw, statusCode, ProtocolVersion)
	}()

	pkt, err := p.rw.ReadMsg()
	if err != nil {
		return fmt.Errorf("peer [%x] failed to read packet: %v", p.ID(), err)
	}
	if pkt.Code != statusCode {
		return fmt.Errorf("peer [%x] sent packet %x before status packet", p.ID(), pkt.Code)
	}
	s := rlp.NewStream(pkt.Payload, uint64(pkt.Size))
	_, err = s.List()
	if err != nil {
		return fmt.Errorf("peer [%x] sent bad status message: %v", p.ID(), err)
	}
	peerVersion, err := s.Uint()
	if err != nil {
		return fmt.Errorf("peer [%x] sent bad status message (unable to decode version): %v", p.ID(), err)
	}
	if peerVersion != ProtocolVersion {
		return fmt.Errorf("peer [%x]: protocol version mismatch %d != %d", p.ID(), peerVersion, ProtocolVersion)
	}
	if err := <-ec; err != nil {
		return fmt.Errorf("peer [%x] failed to send status packet: %v", p.ID(), err)
	}

	return nil
}

func (p *peer) loop(p2pMaxPkgSize uint32) error {

	if err := p.handshake(); err != nil {
		log.Warn("handshake failed", "peer", p.ID(), "err", err)
		return err
	}

	quit := make(chan struct{})
	go p.broadcast(quit)
	defer close(quit)

	for {
		pkt, err := p.rw.ReadMsg()
		if err != nil {
			if err.Error() != "EOF" {
				log.Warn("message loop", "peer", p.ID(), "err", err)
			}
			return err
		}
		if pkt.Size > p2pMaxPkgSize {
			log.Warn("oversized packet received", "peer", p.ID())
			return errors.New("oversized packet received")
		}

		switch pkt.Code {
		case statusCode:
			log.Warn("unxepected status packet received", "peer", p.ID())

		case messagesCode:
			var bs [][]byte

			if err := pkt.Decode(&bs); err != nil {
				log.Warn("failed to decode messages, peer will be disconnected", "peer", p.ID(), "err", err)
				return errors.New("invalid messages")
			}

			for _, b := range bs {
				m := &message{body: b, peerID: p.p2.ID().Bytes()}
				if err := m.validate(); err != nil {
					log.Error("bad message received, peer will be disconnected", "peer", p.ID(), "err", err)
					return errors.New("invalid message")
				}
				p.board.put(m)
			}
		}
	}
}
