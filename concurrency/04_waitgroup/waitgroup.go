package waitgroup

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type WaitGroup struct {
	count uint32
}

func (wg *WaitGroup) Add(delta int) {
	value := atomic.AddUint32(&wg.count, uint32(delta))
	
	if int32(value) < 0 {
		panic("negative WaitGroup counter")
	}

	if value == 0 && delta < 0 {
		futex.WakeAll(&wg.count)
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