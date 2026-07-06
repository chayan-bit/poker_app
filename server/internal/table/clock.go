package table

import "time"

// Clock is the injectable time + timer source for a table loop. Production uses
// realClock (wall time, real timers); tests supply a fake so turn deadlines fire
// deterministically without sleeping.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// NewTimer returns a stopped timer. Arm it with Reset; disarm with Stop.
	// A stopped timer never fires, so the loop's select on it simply blocks.
	NewTimer() Timer
}

// Timer is a single, reusable one-shot deadline. It is owned by the table loop
// goroutine, so it needs no internal synchronization.
type Timer interface {
	// C is the fire channel; a value is delivered when an armed deadline elapses.
	C() <-chan time.Time
	// Reset arms (or re-arms) the timer to fire after d.
	Reset(d time.Duration)
	// Stop disarms the timer; after Stop it will not fire until Reset again.
	Stop()
}

// realClock is the production Clock backed by the standard library.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) NewTimer() Timer {
	t := time.NewTimer(time.Hour)
	if !t.Stop() {
		<-t.C
	}
	return &realTimer{t: t}
}

type realTimer struct{ t *time.Timer }

func (r *realTimer) C() <-chan time.Time { return r.t.C }

func (r *realTimer) Reset(d time.Duration) {
	r.Stop()
	r.t.Reset(d)
}

func (r *realTimer) Stop() {
	if !r.t.Stop() {
		select {
		case <-r.t.C:
		default:
		}
	}
}
