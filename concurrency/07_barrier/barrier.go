package barrier

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type Barrier struct {
	need    uint32
	arrived uint32
	round   uint32
}

func New(n int) *Barrier {
	if n <= 0 || uint64(n) > uint64(^uint32(0)) {
		panic("barrier: invalid number of participants")
	}

	return &Barrier{
		need: uint32(n),
	}
}

func (b *Barrier) Wait() {
	currentRound := atomic.LoadUint32(&b.round)

	count := atomic.AddUint32(&b.arrived, 1)

	if count == b.need {
		atomic.StoreUint32(&b.arrived, 0)
		atomic.AddUint32(&b.round, 1)

		futex.WakeAll(&b.round)

		return
	}

	for atomic.LoadUint32(&b.round) == currentRound {
		futex.Wait(&b.round, currentRound)
	}
}
