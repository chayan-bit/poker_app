package localcore

import (
	"encoding/json"

	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// armTimer sets the current actor's turn deadline relative to the most recently
// observed timestamp. There is no wall clock: the deadline only ever fires when
// a later Tick advances nowMs past it, or an explicit fold_on_timeout arrives.
func (lt *LocalTable) armTimer() {
	lt.turnDeadlineMs = lt.nowMs + lt.cfg.TurnTimeoutMs
}

// Tick advances the table's notion of time to nowMs and processes every deadline
// now due: the current actor's turn timeout and each disconnected seat's grace
// window. It returns the resulting per-recipient event envelopes. Time is purely
// an input; nowMs never regresses.
func (lt *LocalTable) Tick(nowMs int64) map[string][]json.RawMessage {
	o := lt.newOut()
	if nowMs > lt.nowMs {
		lt.nowMs = nowMs
	}

	if lt.turnDeadlineMs != 0 && lt.nowMs >= lt.turnDeadlineMs {
		lt.turnDeadlineMs = 0
		lt.onTurnTimeout(o)
	}

	graceExpired := false
	for _, s := range lt.seats {
		if s.disconnected && s.graceDeadlineMs != 0 && lt.nowMs >= s.graceDeadlineMs {
			s.disconnected = false
			s.graceDeadlineMs = 0
			s.sittingOut = true
			graceExpired = true
		}
	}
	if graceExpired {
		o.broadcast(protocol.EvSeatUpdate, lt.seatUpdatePayload())
	}

	return o.result()
}
