package mutex

import (
	"sync/atomic"

	"primitives/internal/futex"
)

const (
	free = iota
	held
	contended
)

type Mutex struct {
	state uint32
}

func (m *Mutex) Lock() {
	if m.TryLock() {
		return
	}

	for {
		old := atomic.SwapUint32(&m.state, contended)

		if old == free {
			return
		}

		futex.Wait(&m.state, contended)
	}
}

func (m *Mutex) TryLock() bool {
	return atomic.CompareAndSwapUint32(&m.state, free, held);
}

func (m *Mutex) Unlock() {
	old := atomic.SwapUint32(&m.state, free)
	if old == free {
		panic("Unlock of unlocked mutex")
	}

	if old == contended {
		futex.Wake(&m.state)
	}
}
