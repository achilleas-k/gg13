package device

import (
	"time"
)

// A routine runs a function in the background at a fixed interval until
// stopped.
type routine struct {
	stopChan chan bool
}

func newRoutine(fn func(), dt time.Duration) *routine {
	r := &routine{}

	tick := time.Tick(dt)
	r.stopChan = make(chan bool)
	go func() {
		for {
			select {
			case <-r.stopChan:
				return
			case <-tick:
				fn()
			}
		}
	}()

	return r
}

// Stop the routine.
func (r *routine) stop() {
	close(r.stopChan)
}
