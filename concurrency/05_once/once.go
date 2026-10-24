package once

import (
	"primitives/internal/futex"
	"sync/atomic"
)

type Once struct {
	state uint32
}

func (o *Once) Do(f func()) {
	value := atomic.CompareAndSwapUint32(&o.state, 0, 1)
	if !value {
		for {
    		if o.Done() {
        		return
    		}

    		futex.Wait(&o.state, 1)
		}
	}

	defer func() { 
		atomic.StoreUint32(&o.state, 2)
		futex.WakeAll(&o.state)
	}()

	f()
}

func (o *Once) Done() bool {
	state := atomic.LoadUint32(&o.state)
	
	return state == 2
}
