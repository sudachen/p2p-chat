package chat

import (
	"sync"
	"time"
)

// The TimeStamp Filter
type TSF struct {
	mu sync.Mutex
	m  map[Hash]int64
}

func newTSF() *TSF{
	return &TSF{
		m: make(map[Hash]int64),
	}
}

func (tsf *TSF) pass(m *message) bool {
	dt := m.deathTime()
	hash := m.hash()

	tsf.mu.Lock()
	_, exists := tsf.m[hash]
	if !exists {
		tsf.m[hash] = dt
	}
	tsf.mu.Unlock()

	return !exists
}

func (tsf *TSF) passDT(m *message, now int64) bool {
	dt := m.deathTime()
	if dt <= now {
		return false
	}

	return tsf.pass(m)
}

func (tsf *TSF) clean() {
	t := time.Now().Unix()
	tsf.mu.Lock()
	for k, dt := range tsf.m {
		if dt <= t {
			delete(tsf.m, k)
		}
	}
	tsf.mu.Unlock()
}

func (tsf *TSF) expire(quit chan struct{}) {
	t := time.NewTicker(TTL*time.Second)
	for {
		if done2(quit, t.C) {
			return
		}
		tsf.clean()
	}
}
