package chat

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const expireTimeout = 10 * time.Second

type board struct {
	ring  []unsafe.Pointer
	index int64
	mu    sync.RWMutex
	known map[Hash]int64
}

func newBoard(ringSize int) *board {
	return &board{
		known: make(map[Hash]int64),
		ring:  make([]unsafe.Pointer, ringSize),
	}
}

func (brd *board) get(index int64) (*message, int64) {
	L := int64(len(brd.ring))

	for { // trying to fetch message by index
		head := int64(0)
		tail := atomic.LoadInt64(&brd.index)
		if tail > L {
			head = tail - L
		}
		if index < head {
			index = head
		}
		if index >= tail {
			return nil, index
		}
		p := &brd.ring[index%L]
		m := (*message)(atomic.LoadPointer(p))
		if m == nil || m.index < index {
			return nil, index
		}
		if m.index == index {
			return m, index + 1
		}
	}
}

func (brd *board) know(m *message) bool {
	h := m.hash() // can take a time
	brd.mu.RLock()
	_, exists := brd.known[h]
	brd.mu.RUnlock()
	return exists
}

func (brd *board) put(m *message) {
	hash := m.hash()    // can take a time
	dt := m.deathTime() // can take a time

	brd.mu.RLock()
	_, exists := brd.known[hash]
	brd.mu.RUnlock()

	if exists {
		return
	}

	brd.mu.Lock()
	brd.known[hash] = dt
	brd.mu.Unlock()

	L := int64(len(brd.ring))
	tail := atomic.AddInt64(&brd.index, 1)
	m.index = tail - 1
	p := &brd.ring[m.index%L]
	atomic.StorePointer(p, unsafe.Pointer(m))
}

func (brd *board) expire(quit chan struct{}) {
	clock := time.NewTicker(expireTimeout)
	for {
		if done2(quit, clock.C) {
			return
		}
		brd.mu.Lock()
		for k, d := range brd.known {
			if d > time.Now().Unix() {
				delete(brd.known, k)
			}
		}
		brd.mu.Unlock()
	}
}
