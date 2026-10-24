package spinlock

import "sync/atomic"

type Spinlock struct {
	locked atomic.Bool
}

func (s *Spinlock) Lock() {
	for !s.locked.CompareAndSwap(false, true) {
	}
}

func (s *Spinlock) TryLock() bool {
	return s.locked.CompareAndSwap(false, true)
}

func (s *Spinlock) Unlock() {
	old := s.locked.Swap(false)

	if !old {
		panic("Unlock of unlocked spinlock")
	}
}

type TTAS struct {
	locked atomic.Bool
}

func (s *TTAS) Lock() {
	for {
		if !s.locked.Load() {
			success := s.locked.CompareAndSwap(false, true)

			if success {
				return
			}
		}
	}
}

func (s *TTAS) TryLock() bool {
	return s.locked.CompareAndSwap(false, true)
}

func (s *TTAS) Unlock() {
	old := s.locked.Swap(false)

	if !old {
		panic("Unlock of unlocked spinlock")
	}
}
