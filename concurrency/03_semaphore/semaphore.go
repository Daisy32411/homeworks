package semaphore

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type Semaphore struct {
	permits uint32
}

func New(n int) *Semaphore {
	if n < 0 || uint64(n) > uint64(^uint32(0)) {
		panic("semaphore: invalid number of permits")
	}

	return &Semaphore{
		permits: uint32(n),
	}
}

func (s *Semaphore) Acquire() {
	for {
		if s.TryAcquire() {
			return
		}

		futex.Wait(&s.permits, 0)
	}
}

func (s *Semaphore) TryAcquire() bool {
	for {
		permis := atomic.LoadUint32(&s.permits)

		if permis == 0 {
			return false
		}

		success := atomic.CompareAndSwapUint32(&s.permits, permis, permis - 1)

		if success {
			return true
		}
	}
}

func (s *Semaphore) Release() {
	atomic.AddUint32(&s.permits, 1)
	futex.Wake(&s.permits)
}

func (s *Semaphore) Available() int {
	return int(atomic.LoadUint32(&s.permits))
}