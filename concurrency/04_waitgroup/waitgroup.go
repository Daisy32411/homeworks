package waitgroup

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type WaitGroup struct {
	count uint32
}

func (wg *WaitGroup) Add(delta int) {
	change := int64(delta)
	max := int64(^uint32(0))

	for {
		old := atomic.LoadUint32(&wg.count)

		if change < -int64(old) || change > max-int64(old) {
			panic("waitgroup: counter out of range")
		}

		next := uint32(int64(old) + change)

		if atomic.CompareAndSwapUint32(&wg.count, old, next) {
			if old != 0 && next == 0 {
				futex.WakeAll(&wg.count)
			}

			return
		}
	}
}

func (wg *WaitGroup) Done() {
	wg.Add(-1)
}

func (wg *WaitGroup) Wait() {
	for {
		value := atomic.LoadUint32(&wg.count)

		if value == 0 {
			return
		}

		futex.Wait(&wg.count, value)
	}
}