package chat

import (
	"sync"

	"github.com/ethereum/go-ethereum/log"
	"time"
)

const expireTimeout = 10 * time.Second

type board struct {
	mu         sync.Mutex
	ring       []*message
	head, tail uint64
	mu2        sync.Mutex
	known      map[Hash]int64
}

func newBoard(ringSize int) *board {
	return &board{
		known: make(map[Hash]int64),
		ring:  make([]*message, ringSize),
	}
}

func (brd *board) get(index uint64) (m *message, newIndex uint64) {
	brd.mu.Lock()
	defer brd.mu.Unlock()
	newIndex = index
	if brd.head > newIndex {
		newIndex = brd.head
	}
	if newIndex < brd.tail {
		m = brd.ring[newIndex%uint64(len(brd.ring))]
		newIndex += 1
	}
	return
}

func (brd *board) know(m *message) bool {
	h := m.hash() // can take a time
	brd.mu2.Lock()
	_, exists := brd.known[h]
	brd.mu2.Unlock()
	return exists
}

func (brd *board) put(m *message) {
	hash := m.hash()    // can take a time
	dt := m.deathTime() // can take a time

	brd.mu2.Lock()
	_, exists := brd.known[hash]
	if !exists {
		brd.known[hash] = dt
	}
	brd.mu2.Unlock()
	if exists {
		return
	}

	brd.mu.Lock()
	L := uint64(len(brd.ring))
	index := brd.tail%L
	if brd.tail == brd.head+L {
		brd.head += 1
	}

	log.Trace("put to ring", "hash", m.hash(), "index", brd.tail, "hash")
	brd.ring[index] = m
	brd.tail += 1
	brd.mu.Unlock()
}

func (brd *board) expire(quit chan struct{}) {
	clock := time.NewTicker(expireTimeout)
	for {
		if done2(quit, clock.C) {
			return
		}
		brd.mu2.Lock()
		for k, d := range brd.known {
			if d > time.Now().Unix() {
				delete(brd.known, k)
			}
		}
		brd.mu2.Unlock()
	}
}
