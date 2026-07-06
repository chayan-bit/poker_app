package localcore

import (
	"encoding/json"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// VoidHand aborts the in-flight hand and returns every committed chip to the
// stacks it came from, resetting the table to the between-hands state. It is the
// recovery path for the distributed design (issue #27): when the dealing peer
// drops mid-hand, the surviving peers void the hand rather than settling a
// partial pot. It is a no-op (empty result) when no hand is running.
//
// Committed chips are returned by restoring each dealt seat's stack to the value
// snapshotted at hand start (recordStartStacks), which by construction equals
// "current stack + everything this seat put in the pot".
func (lt *LocalTable) VoidHand() map[string][]json.RawMessage {
	o := lt.newOut()
	if lt.hand == nil {
		return o.result()
	}

	for seat, start := range lt.startStack {
		if s, ok := lt.seats[seat]; ok {
			s.stack = start
		}
	}

	lt.hand = nil
	lt.turnDeadlineMs = 0
	lt.commitment = ""
	lt.seedHex = ""
	lt.handID = ""
	lt.startStack = map[int]engine.Chips{}

	o.broadcast(protocol.EvSeatUpdate, lt.seatUpdatePayload())
	o.broadcast(protocol.EvTableStatus, tableStatus{
		TableID: lt.cfg.ID, WaitingForHost: !lt.autoStart() && !lt.hostStarted, SeatedCount: len(lt.seats),
	})
	return o.result()
}
