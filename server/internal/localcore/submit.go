package localcore

import (
	"encoding/json"
	"fmt"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// CmdFoldOnTimeout is the localcore-only command that expresses a turn timeout
// as an explicit, log-driven input carrying the coordinator's timestamp. It is
// additive to the protocol commands (issue #27 distributed scope): the online WS
// path derives timeouts from its injectable clock, whereas offline peers must
// agree on the exact timestamp, so the fold-on-timeout is a replicated command.
const CmdFoldOnTimeout = "fold_on_timeout"

// actionKinds maps wire kind strings to engine action kinds.
var actionKinds = map[string]engine.ActionKind{
	"fold":  engine.Fold,
	"check": engine.Check,
	"call":  engine.Call,
	"bet":   engine.Bet,
	"raise": engine.Raise,
}

// Submit applies one client command from playerID and returns the resulting
// per-recipient event envelopes. A malformed envelope returns an error and no
// events; a valid command that is rejected by game rules returns an error event
// addressed to the actor (never to the whole table).
func (lt *LocalTable) Submit(playerID string, envelopeJSON []byte) (map[string][]json.RawMessage, error) {
	var env protocol.Envelope
	if err := json.Unmarshal(envelopeJSON, &env); err != nil {
		return nil, fmt.Errorf("localcore: invalid envelope: %w", err)
	}
	o := lt.newOut()
	lt.dispatch(o, playerID, env)
	return o.result(), nil
}

// dispatch routes one decoded command to its handler.
func (lt *LocalTable) dispatch(o *out, playerID string, env protocol.Envelope) {
	switch env.Type {
	case protocol.CmdJoinTable:
		lt.handleJoin(o, playerID)
	case protocol.CmdSitDown:
		lt.handleSitDown(o, playerID, env.Data)
	case protocol.CmdPlaceBet:
		lt.handleBet(o, playerID, env.Data)
	case protocol.CmdResync:
		o.sendTo(playerID, protocol.EvSnapshot, lt.snapshotFor(playerID))
	case protocol.CmdStartHand:
		lt.handleStartHand(o, playerID)
	case protocol.CmdRebuy:
		lt.handleRebuy(o, playerID, env.Data)
	case protocol.CmdSitOut:
		lt.handleSitOut(o, playerID)
	case protocol.CmdSitIn:
		lt.handleSitIn(o, playerID)
	case protocol.CmdDisconnected:
		lt.handleDisconnect(o, playerID)
	case CmdFoldOnTimeout:
		lt.handleFoldOnTimeout(o, env.Data)
	default:
		o.sendError(playerID, "unknown_command", "unrecognized command type")
	}
}

// handleJoin subscribes a connection and sends it a personalized snapshot. A
// reclaimed seat after a disconnect clears its disconnected flag.
func (lt *LocalTable) handleJoin(o *out, playerID string) {
	lt.connected[playerID] = true
	if seat, ok := lt.seatOf(playerID); ok {
		s := lt.seats[seat]
		if s.disconnected {
			s.disconnected = false
			s.graceDeadlineMs = 0
			o.broadcast(protocol.EvSeatUpdate, lt.seatUpdatePayload())
		}
	}
	o.sendTo(playerID, protocol.EvSnapshot, lt.snapshotFor(playerID))
}

// handleSitDown validates a buy-in and seats the player. Offline play uses
// per-session fun chips, so there is no ledger debit (issue #27 scope).
func (lt *LocalTable) handleSitDown(o *out, playerID string, data json.RawMessage) {
	var sit cmdSitDown
	if err := json.Unmarshal(data, &sit); err != nil {
		o.sendError(playerID, "bad_request", "invalid sit_down payload")
		return
	}
	if _, taken := lt.seats[sit.Seat]; taken {
		o.sendError(playerID, "seat_taken", "that seat is occupied")
		return
	}
	if lt.cfg.MaxSeats > 0 && (sit.Seat < 0 || sit.Seat >= lt.cfg.MaxSeats) {
		o.sendError(playerID, "bad_seat", "seat out of range")
		return
	}
	if !lt.buyInInRange(engine.Chips(sit.BuyIn)) {
		o.sendError(playerID, "bad_buyin", "buy-in outside table range")
		return
	}
	lt.seats[sit.Seat] = &seatState{playerID: playerID, stack: engine.Chips(sit.BuyIn)}
	lt.connected[playerID] = true
	if lt.cfg.HostPlayerID == "" {
		lt.cfg.HostPlayerID = playerID
	}
	o.broadcast(protocol.EvSeatUpdate, lt.seatUpdatePayload())
	lt.startHandIfReady(o)
}

// buyInInRange enforces at least one big blind and at most 1000 big blinds.
func (lt *LocalTable) buyInInRange(amt engine.Chips) bool {
	if amt <= 0 {
		return false
	}
	min := engine.Chips(lt.cfg.BigBlind)
	if min <= 0 {
		min = 1
	}
	return amt >= min && amt <= 1000*min
}

// handleBet applies a betting action through the pure engine.
func (lt *LocalTable) handleBet(o *out, playerID string, data json.RawMessage) {
	var pb protocol.PlaceBet
	if err := json.Unmarshal(data, &pb); err != nil {
		o.sendError(playerID, "bad_request", "invalid place_bet payload")
		return
	}
	kind, ok := actionKinds[pb.Kind]
	if !ok {
		o.sendError(playerID, "illegal_action", "unknown action kind")
		return
	}
	seat, seated := lt.seatOf(playerID)
	if !seated || lt.hand == nil {
		o.sendError(playerID, "illegal_action", "not in an active hand")
		return
	}
	act := engine.Action{SeatID: seat, Kind: kind, Amount: engine.Chips(pb.Amount)}
	if err := lt.applyAction(o, seat, act, pb.Kind); err != nil {
		o.sendError(playerID, "illegal_action", err.Error())
	}
}

// handleStartHand honors a host-only start_hand in a non-auto-start room.
func (lt *LocalTable) handleStartHand(o *out, playerID string) {
	if playerID != lt.cfg.HostPlayerID {
		o.sendError(playerID, "not_host", "only the host can start the hand")
		return
	}
	lt.hostStarted = true
	lt.startHandIfReady(o)
}

// handleRebuy adds chips to a seat between hands. Offline uses fun chips: the
// resulting stack must land within [BigBlind, 1000*BigBlind]; no ledger debit.
func (lt *LocalTable) handleRebuy(o *out, playerID string, data json.RawMessage) {
	var rb cmdRebuy
	if err := json.Unmarshal(data, &rb); err != nil {
		o.sendError(playerID, "bad_request", "invalid rebuy payload")
		return
	}
	seat, seated := lt.seatOf(playerID)
	if !seated {
		o.sendError(playerID, "not_seated", "rebuy requires a seat")
		return
	}
	if lt.hand != nil {
		o.sendError(playerID, "hand_in_progress", "rebuy only allowed between hands")
		return
	}
	s := lt.seats[seat]
	newStack := s.stack + engine.Chips(rb.Amount)
	min := engine.Chips(lt.cfg.BigBlind)
	if min <= 0 {
		min = 1
	}
	if rb.Amount <= 0 || newStack < min || newStack > 1000*min {
		o.sendError(playerID, "bad_rebuy", "rebuy would put stack outside table range")
		return
	}
	s.stack = newStack
	s.sittingOut = false
	o.broadcast(protocol.EvSeatUpdate, lt.seatUpdatePayload())
	lt.startHandIfReady(o)
}

// handleSitOut marks a seat sitting out from the next hand, folding immediately
// (their turn) or when action reaches them (foldPending).
func (lt *LocalTable) handleSitOut(o *out, playerID string) {
	seat, seated := lt.seatOf(playerID)
	if !seated {
		o.sendError(playerID, "not_seated", "sit_out requires a seat")
		return
	}
	s := lt.seats[seat]
	s.sittingOut = true
	if lt.hand != nil {
		if i := lt.handIndex(seat); i >= 0 {
			p := lt.hand.Players[i]
			if p.Status == engine.Active || p.Status == engine.AllIn {
				if lt.hand.ToActPos == i {
					_ = lt.applyAction(o, seat, engine.Action{SeatID: seat, Kind: engine.Fold}, "fold")
				} else {
					s.foldPending = true
				}
			}
		}
	}
	o.broadcast(protocol.EvSeatUpdate, lt.seatUpdatePayload())
}

// handleSitIn clears a seat's sitting-out state so it is dealt into the next hand.
func (lt *LocalTable) handleSitIn(o *out, playerID string) {
	seat, seated := lt.seatOf(playerID)
	if !seated {
		o.sendError(playerID, "not_seated", "sit_in requires a seat")
		return
	}
	s := lt.seats[seat]
	s.sittingOut = false
	s.foldPending = false
	o.broadcast(protocol.EvSeatUpdate, lt.seatUpdatePayload())
	lt.startHandIfReady(o)
}

// handleDisconnect unsubscribes the player; a seated player starts the
// disconnect-grace window (measured from the last observed timestamp). It does
// NOT fold or unseat.
func (lt *LocalTable) handleDisconnect(o *out, playerID string) {
	delete(lt.connected, playerID)
	if seat, ok := lt.seatOf(playerID); ok {
		s := lt.seats[seat]
		if !s.disconnected {
			s.disconnected = true
			s.graceDeadlineMs = lt.nowMs + lt.cfg.DisconnectGraceMs
			o.broadcast(protocol.EvSeatUpdate, lt.seatUpdatePayload())
		}
	}
}

// handleFoldOnTimeout applies the current actor's turn timeout as an explicit
// command carrying the coordinator's timestamp. It advances nowMs to that
// timestamp so subsequent grace-window checks stay consistent, then auto-acts.
func (lt *LocalTable) handleFoldOnTimeout(o *out, data json.RawMessage) {
	var to cmdTimeout
	if len(data) > 0 {
		_ = json.Unmarshal(data, &to)
	}
	if to.NowMs > lt.nowMs {
		lt.nowMs = to.NowMs
	}
	lt.onTurnTimeout(o)
}
