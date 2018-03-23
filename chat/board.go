package chat

import (
	"sync/atomic"
	"unsafe"
)

type board struct {
	index int64
	ring  []unsafe.Pointer
	tsf   *TSF
}

func newBoard(ringSize int) *board {
	return &board{
		ring:  make([]unsafe.Pointer, ringSize),
		tsf:   newTSF(),
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

func (brd *board) put(m *message) {

	if !brd.tsf.pass(m) {
		return
	}

	L := int64(len(brd.ring))
	tail := atomic.AddInt64(&brd.index, 1)
	m.index = tail - 1
	p := &brd.ring[m.index%L]
	atomic.StorePointer(p, unsafe.Pointer(m))
}

func (brd *board) expire(quit chan struct{}) {
	brd.tsf.expire(quit)
}
