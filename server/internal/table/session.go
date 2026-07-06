package table

import (
	"encoding/json"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// handleStartHand honors a host-only start_hand in a non-auto-start room. Once
// accepted, hostStarted latches so subsequent hands auto-continue.
func (t *Table) handleStartHand(cmd Command) {
	if cmd.PlayerID != t.Cfg.HostPlayerID {
		t.sendError(cmd.Reply, "not_host", "only the host can start the hand")
		return
	}
	t.hostStarted = true
	t.startHandIfReady()
}

// cmdRebuy is the decoded body of a rebuy command.
type cmdRebuy struct {
	TableID string `json:"tableId"`
	Amount  int64  `json:"amount"`
}

// handleRebuy adds chips to a seat between hands. The resulting stack must land
// within [BigBlind, 1000*BigBlind]; the chips are debited via the ledger.
func (t *Table) handleRebuy(cmd Command) {
	var rb cmdRebuy
	if err := json.Unmarshal(cmd.Msg.Data, &rb); err != nil {
		t.sendError(cmd.Reply, "bad_request", "invalid rebuy payload")
		return
	}
	seat, seated := t.seatOf(cmd.PlayerID)
	if !seated {
		t.sendError(cmd.Reply, "not_seated", "rebuy requires a seat")
		return
	}
	if t.Hand != nil {
		t.sendError(cmd.Reply, "hand_in_progress", "rebuy only allowed between hands")
		return
	}
	if t.Cfg.Tournament != nil && t.Cfg.Tournament.NoRebuy {
		t.sendError(cmd.Reply, "no_rebuy", "rebuys are not allowed in this tournament")
		return
	}
	s := t.seats[seat]
	newStack := s.stack + engine.Chips(rb.Amount)
	min := t.Cfg.BigBlind
	if min <= 0 {
		min = 1
	}
	max := 1000 * min
	if rb.Amount <= 0 || newStack < min || newStack > max {
		t.sendError(cmd.Reply, "bad_rebuy", "rebuy would put stack outside table range")
		return
	}
	if err := t.deps.Ledger.BuyIn(cmd.PlayerID, engine.Chips(rb.Amount)); err != nil {
		code := "buyin_failed"
		if err == economy.ErrInsufficientFunds {
			code = "insufficient_funds"
		}
		t.sendError(cmd.Reply, code, err.Error())
		return
	}
	s.stack = newStack
	// A funded seat is no longer broke and opts back in.
	s.brokeAtHand = 0
	s.sittingOut = false
	t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
	t.startHandIfReady()
}

// handleSitOut marks a seat sitting out from the next hand. If it is mid-hand
// and their turn, they fold immediately; if mid-hand but not their turn, they
// fold when action reaches them (foldPending, drained by driveAutoFolds).
func (t *Table) handleSitOut(cmd Command) {
	seat, seated := t.seatOf(cmd.PlayerID)
	if !seated {
		t.sendError(cmd.Reply, "not_seated", "sit_out requires a seat")
		return
	}
	s := t.seats[seat]
	s.sittingOut = true

	if t.Hand != nil {
		if i := t.handIndex(seat); i >= 0 {
			p := t.Hand.Players[i]
			if p.Status == engine.Active || p.Status == engine.AllIn {
				if t.Hand.ToActPos == i {
					_ = t.applyAction(seat, engine.Action{SeatID: seat, Kind: engine.Fold}, "fold")
				} else {
					s.foldPending = true
				}
			}
		}
	}
	t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
}

// handleSitIn clears a seat's sitting-out state (including a timeout- or
// grace-induced sit-out) so it is dealt into the next hand.
func (t *Table) handleSitIn(cmd Command) {
	seat, seated := t.seatOf(cmd.PlayerID)
	if !seated {
		t.sendError(cmd.Reply, "not_seated", "sit_in requires a seat")
		return
	}
	s := t.seats[seat]
	s.sittingOut = false
	s.foldPending = false
	t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
	t.startHandIfReady()
}

// handleDisconnect is submitted by the gateway when a connection's socket
// closes. It unsubscribes the player; if they are seated it starts the
// disconnect-grace window (the normal turn timer keeps running). It does NOT
// fold or unseat.
func (t *Table) handleDisconnect(cmd Command) {
	delete(t.subs, cmd.PlayerID)
	if seat, ok := t.seatOf(cmd.PlayerID); ok {
		s := t.seats[seat]
		if !s.disconnected {
			s.disconnected = true
			s.graceDeadline = t.deps.Now().Add(t.deps.DisconnectGrace)
			t.rearm()
			t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
		}
	}
	t.refreshIdle()
}

// broadcastStatus emits a table_status so private-room clients can show a start
// button while the room waits for its host to deal the first hand.
func (t *Table) broadcastStatus(waitingForHost bool) {
	t.broadcast(protocol.EvTableStatus, tableStatus{
		TableID:        t.Cfg.ID,
		WaitingForHost: waitingForHost,
		SeatedCount:    len(t.seats),
	})
}
