package rwmutex

import (
	"primitives/internal/futex"
	"sync/atomic"
)

const writer = 1 << 31

type RWMutex struct {
	state uint32
}

func (rw *RWMutex) RLock() {
	for {
		state := atomic.LoadUint32(&rw.state)

		if state & writer != 0 {
			futex.Wait(&rw.state, state)
			continue
		}

		if atomic.CompareAndSwapUint32(&rw.state, state, state + 1) {
			return
		}
	}
}

func (rw *RWMutex) RUnlock() {
	for {
		state := atomic.LoadUint32(&rw.state)

		if state == 0 || state & writer != 0 {
			panic("runlock of unlocked rwmutex")
		}

		if atomic.CompareAndSwapUint32(&rw.state, state, state - 1) {
			if state == 1 {
				futex.WakeAll(&rw.state)
			}

			return
		}
	}
}

func (rw *RWMutex) Lock() {
	for {
		success := atomic.CompareAndSwapUint32(&rw.state, 0, writer)

		if success {
			return
		}

		state := atomic.LoadUint32(&rw.state)

		if state != 0 {
			futex.Wait(&rw.state, state)
		}
	}
}

func (rw *RWMutex) Unlock() {
	if !atomic.CompareAndSwapUint32(&rw.state, writer, 0) {
		panic("unlock of unlocked rwmutex")
	}

	futex.WakeAll(&rw.state)
}