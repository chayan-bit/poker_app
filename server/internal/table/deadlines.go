package table

import (
	"time"

	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// This file owns the table's time-based scheduling. A table needs several
// concurrent deadlines - the current actor's turn, each disconnected seat's
// grace window, and the empty-table idle shutdown - but the loop multiplexes a
// single injectable Timer. We keep every deadline as an absolute time on the
// Table and, whenever the set changes, arm the one Timer to the earliest of
// them (rearm). When it fires, onTimer processes every deadline now due. This
// stays deterministic under the fake clock: a test advances Now() past a
// deadline and fires the timer.

// armTimer sets the current actor's turn deadline and re-arms the loop timer.
func (t *Table) armTimer() {
	t.turnDeadline = t.deps.Now().Add(t.deps.TurnTimeout)
	t.rearm()
}

// stopTimer clears the turn deadline (no hand in progress or hand ended).
func (t *Table) stopTimer() {
	t.turnDeadline = time.Time{}
	t.rearm()
}

// refreshIdle arms or clears the empty-table idle deadline: a table with no
// subscribers and no seated players shuts down after Deps.IdleTimeout.
func (t *Table) refreshIdle() {
	if len(t.subs) == 0 && len(t.seats) == 0 {
		if t.idleDeadline.IsZero() {
			t.idleDeadline = t.deps.Now().Add(t.deps.IdleTimeout)
		}
	} else {
		t.idleDeadline = time.Time{}
	}
	t.rearm()
}

// rearm points the single loop timer at the earliest active deadline, or stops
// it when none are set.
func (t *Table) rearm() {
	var next time.Time
	consider := func(dl time.Time) {
		if dl.IsZero() {
			return
		}
		if next.IsZero() || dl.Before(next) {
			next = dl
		}
	}
	consider(t.turnDeadline)
	consider(t.idleDeadline)
	for _, s := range t.seats {
		if s.disconnected {
			consider(s.graceDeadline)
		}
	}
	if next.IsZero() {
		t.timer.Stop()
		return
	}
	d := next.Sub(t.deps.Now())
	if d < 0 {
		d = 0
	}
	t.timer.Reset(d)
}

// onTimer runs when the loop timer fires: it dispatches every deadline that is
// now due (turn, disconnect grace, idle), then re-arms for the next one.
func (t *Table) onTimer() {
	now := t.deps.Now()

	if !t.turnDeadline.IsZero() && !now.Before(t.turnDeadline) {
		t.turnDeadline = time.Time{}
		t.onTurnTimeout()
	}

	graceExpired := false
	for _, s := range t.seats {
		if s.disconnected && !s.graceDeadline.IsZero() && !now.Before(s.graceDeadline) {
			s.disconnected = false
			s.graceDeadline = time.Time{}
			s.sittingOut = true
			graceExpired = true
		}
	}
	if graceExpired {
		t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
	}

	if !t.idleDeadline.IsZero() && !now.Before(t.idleDeadline) {
		t.shutdown()
		return
	}

	t.rearm()
}

// shutdown stops the table: disarm the timer, notify the registry to remove it,
// and mark the loop to exit on return.
func (t *Table) shutdown() {
	t.timer.Stop()
	if t.deps.OnShutdown != nil {
		t.deps.OnShutdown(t.Cfg.ID)
	}
	t.stopped = true
}
