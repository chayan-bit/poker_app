package table

import (
	"encoding/json"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// actionKinds maps wire kind strings to engine action kinds.
var actionKinds = map[string]engine.ActionKind{
	"fold":  engine.Fold,
	"check": engine.Check,
	"call":  engine.Call,
	"bet":   engine.Bet,
	"raise": engine.Raise,
}

// handleJoin subscribes a connection and sends it a personalized snapshot. If
// the player is reclaiming a seat after a disconnect, its disconnected flag is
// cleared and the personalized snapshot restores their own hole cards.
func (t *Table) handleJoin(cmd Command) {
	t.subs[cmd.PlayerID] = cmd.Reply
	if seat, ok := t.seatOf(cmd.PlayerID); ok {
		s := t.seats[seat]
		if s.disconnected {
			s.disconnected = false
			s.graceDeadline = time.Time{}
			t.rearm()
			t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
		}
	}
	t.sendTo(cmd.Reply, protocol.EvSnapshot, t.snapshotFor(cmd.PlayerID))
	t.refreshIdle()
}

// handleSitDown validates a buy-in, debits the ledger, seats the player, and
// broadcasts the seat change. It then tries to auto-start a hand.
func (t *Table) handleSitDown(cmd Command) {
	var sit cmdSitDown
	if err := json.Unmarshal(cmd.Msg.Data, &sit); err != nil {
		t.sendError(cmd.Reply, "bad_request", "invalid sit_down payload")
		return
	}
	if _, taken := t.seats[sit.Seat]; taken {
		t.sendError(cmd.Reply, "seat_taken", "that seat is occupied")
		return
	}
	if t.Cfg.MaxSeats > 0 && (sit.Seat < 0 || sit.Seat >= t.Cfg.MaxSeats) {
		t.sendError(cmd.Reply, "bad_seat", "seat out of range")
		return
	}
	if !t.buyInInRange(engine.Chips(sit.BuyIn)) {
		t.sendError(cmd.Reply, "bad_buyin", "buy-in outside table range")
		return
	}
	if err := t.deps.Ledger.BuyIn(cmd.PlayerID, engine.Chips(sit.BuyIn)); err != nil {
		code := "buyin_failed"
		if err == economy.ErrInsufficientFunds {
			code = "insufficient_funds"
		}
		t.sendError(cmd.Reply, code, err.Error())
		return
	}
	t.seats[sit.Seat] = &seatState{playerID: cmd.PlayerID, stack: engine.Chips(sit.BuyIn)}
	if _, ok := t.subs[cmd.PlayerID]; !ok {
		t.subs[cmd.PlayerID] = cmd.Reply
	}
	// First player to sit becomes host if the lobby did not set one.
	if t.Cfg.HostPlayerID == "" {
		t.Cfg.HostPlayerID = cmd.PlayerID
	}
	t.refreshIdle()
	t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
	t.startHandIfReady()
}

// buyInInRange enforces the table's buy-in bounds: at least one big blind, at
// most 1000 big blinds (a sane cap in the absence of per-table overrides).
func (t *Table) buyInInRange(amt engine.Chips) bool {
	if amt <= 0 {
		return false
	}
	min := t.Cfg.BigBlind
	if min <= 0 {
		min = 1
	}
	max := 1000 * min
	return amt >= min && amt <= max
}

// handleBet applies a betting action through the pure engine and broadcasts the
// result. Server is authoritative: illegal actions are rejected here and an
// error is returned only to the actor, never to the whole table.
func (t *Table) handleBet(cmd Command) {
	var pb protocol.PlaceBet
	if err := json.Unmarshal(cmd.Msg.Data, &pb); err != nil {
		t.sendError(cmd.Reply, "bad_request", "invalid place_bet payload")
		return
	}
	kind, ok := actionKinds[pb.Kind]
	if !ok {
		t.sendError(cmd.Reply, "illegal_action", "unknown action kind")
		return
	}
	seat, seated := t.seatOf(cmd.PlayerID)
	if !seated || t.Hand == nil {
		t.sendError(cmd.Reply, "illegal_action", "not in an active hand")
		return
	}
	act := engine.Action{SeatID: seat, Kind: kind, Amount: engine.Chips(pb.Amount)}
	if err := t.applyAction(seat, act, pb.Kind); err != nil {
		t.sendError(cmd.Reply, "illegal_action", err.Error())
	}
}

// handleLeave unseats a player. Mid-hand it folds them (uncontested pots settle
// immediately), then cashes out their remaining stack and unsubscribes.
func (t *Table) handleLeave(cmd Command) {
	seat, seated := t.seatOf(cmd.PlayerID)
	if !seated {
		delete(t.subs, cmd.PlayerID)
		return
	}
	cashOut := t.seats[seat].stack

	if t.Hand != nil {
		if i := t.handIndex(seat); i >= 0 {
			p := t.Hand.Players[i]
			if p.Status == engine.Active || p.Status == engine.AllIn {
				cashOut = p.Stack // committed chips stay in the pot
				t.foldSeat(seat, i)
			}
		}
	}

	delete(t.seats, seat)
	delete(t.subs, cmd.PlayerID)
	t.deps.Ledger.CashOut(cmd.PlayerID, cashOut)
	t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
	t.driveAutoFolds()
	t.refreshIdle()
}

// foldSeat removes a leaving player from the live hand. If it is their turn the
// fold flows through the pure engine (advancing action / settling naturally);
// otherwise the seat is marked folded in place and, if that leaves the pot
// uncontested, the hand is run to showdown and settled.
func (t *Table) foldSeat(seat, idx int) {
	if t.Hand.ToActPos == idx {
		_ = t.applyAction(seat, engine.Action{SeatID: seat, Kind: engine.Fold}, "fold")
		return
	}
	// Mutate the table's own hand copy (unique backing array from Apply/NewHand;
	// single-writer goroutine). The engine stays pure; this is server bookkeeping.
	t.Hand.Players[idx].Status = engine.Folded
	t.broadcast(protocol.EvBetPlaced, betPlaced{
		Seat: seat, Kind: "fold", Amount: 0, Pot: int64(t.Hand.Pot), ToAct: t.toActSeat(),
		CurrentBet: t.currentBet(), ToCall: t.toCall(),
	})
	if done, _ := t.Hand.IsUncontested(); done {
		t.Hand.Street = engine.Showdown
		t.settle()
	}
}

// handIndex returns the index of seat within the live hand, or -1.
func (t *Table) handIndex(seat int) int {
	if t.Hand == nil {
		return -1
	}
	for i, p := range t.Hand.Players {
		if p.SeatID == seat {
			return i
		}
	}
	return -1
}

// applyAction runs one action through the engine and emits the resulting
// events (bet_placed, street_advanced, showdown). It returns the engine error
// unchanged so the caller can report it to the actor only.
func (t *Table) applyAction(seat int, act engine.Action, kind string) error {
	prevStreet := t.Hand.Street
	prevBoard := len(t.Hand.Board)

	nh, err := t.Hand.Apply(act)
	if err != nil {
		return err
	}
	t.Hand = &nh
	if t.rec != nil {
		t.rec.OnAction(streetName(prevStreet), seat, kind, act.Amount)
	}

	t.broadcast(protocol.EvBetPlaced, betPlaced{
		Seat: seat, Kind: kind, Amount: int64(act.Amount),
		Pot: int64(nh.Pot), ToAct: t.toActSeat(),
		CurrentBet: t.currentBet(), ToCall: t.toCall(),
	})

	if nh.Street != prevStreet && nh.Street != engine.Showdown {
		newCards := cardsToStrings(nh.Board[prevBoard:])
		if t.rec != nil {
			t.rec.OnStreet(streetName(nh.Street), newCards)
		}
		t.broadcast(protocol.EvStreet, streetAdvanced{
			Street: streetName(nh.Street), Board: cardsToStrings(nh.Board),
		})
	}

	if nh.Street == engine.Showdown {
		t.settle()
	} else {
		t.armTimer()
	}
	t.driveAutoFolds()
	return nil
}

// driveAutoFolds folds any seat that requested sit_out mid-hand once action
// reaches it. It re-enters applyAction, so a guard prevents unbounded recursion;
// the outer loop chains consecutive auto-folds until the actor is a live player.
func (t *Table) driveAutoFolds() {
	if t.autoFolding {
		return
	}
	t.autoFolding = true
	defer func() { t.autoFolding = false }()
	for t.Hand != nil && t.Hand.Street != engine.Showdown {
		seat := t.toActSeat()
		if seat < 0 {
			return
		}
		s, ok := t.seats[seat]
		if !ok || !s.foldPending {
			return
		}
		if t.handIndex(seat) < 0 {
			return
		}
		_ = t.applyAction(seat, engine.Action{SeatID: seat, Kind: engine.Fold}, "fold")
	}
}

// currentBet is the highest committed on the current street, or 0 with no hand.
func (t *Table) currentBet() int64 {
	if t.Hand == nil || t.Hand.Street == engine.Showdown {
		return 0
	}
	return int64(t.Hand.CurrentBet)
}

// toCall is the chips the seat now to act must add to call, floored at 0.
func (t *Table) toCall() int64 {
	if t.Hand == nil || t.Hand.Street == engine.Showdown {
		return 0
	}
	p := t.Hand.Players[t.Hand.ToActPos]
	d := int64(t.Hand.CurrentBet - p.Committed)
	if d < 0 {
		d = 0
	}
	return d
}

// onTurnTimeout auto-acts for the seat on the clock: check if legal, else fold,
// and mark that seat sitting-out for the next hand.
func (t *Table) onTurnTimeout() {
	if t.Hand == nil {
		return
	}
	idx := t.Hand.ToActPos
	seat := t.Hand.Players[idx].SeatID
	kind := engine.Fold
	kindStr := "fold"
	if t.Hand.Players[idx].Committed == t.Hand.CurrentBet {
		kind, kindStr = engine.Check, "check"
	}
	if s, ok := t.seats[seat]; ok {
		s.sittingOut = true
	}
	_ = t.applyAction(seat, engine.Action{SeatID: seat, Kind: kind}, kindStr)
}

// toActSeat returns the seat currently to act, or -1 when no hand is running.
func (t *Table) toActSeat() int {
	if t.Hand == nil || t.Hand.Street == engine.Showdown {
		return -1
	}
	return t.Hand.Players[t.Hand.ToActPos].SeatID
}

// snapshot builds the full public table view (no players' hole cards).
func (t *Table) snapshot() tableSnapshot {
	snap := tableSnapshot{
		TableID:    t.Cfg.ID,
		Seats:      t.seatViews(),
		Button:     t.button,
		ToAct:      t.toActSeat(),
		Street:     "none",
		CurrentBet: t.currentBet(),
		YourSeat:   -1,
	}
	if t.Hand != nil {
		snap.HandRunning = true
		snap.HandID = t.handID
		snap.Street = streetName(t.Hand.Street)
		snap.Board = cardsToStrings(t.Hand.Board)
		snap.Pot = int64(t.Hand.Pot)
	}
	return snap
}

// snapshotFor is a personalized snapshot: identical to snapshot but, when a hand
// is running and playerID is dealt into it, includes that player's own hole
// cards. This is the reclaim path (rejoin after a disconnect) and is safe for
// any joiner: it only ever reveals the recipient's own cards.
func (t *Table) snapshotFor(playerID string) tableSnapshot {
	snap := t.snapshot()
	seat, ok := t.seatOf(playerID)
	if !ok || t.Hand == nil {
		return snap
	}
	i := t.handIndex(seat)
	if i < 0 {
		return snap
	}
	p := t.Hand.Players[i]
	if p.Status == engine.Folded {
		snap.YourSeat = seat
		return snap
	}
	snap.YourSeat = seat
	snap.YourHole = []string{p.Hole[0].String(), p.Hole[1].String()}
	return snap
}

// seatUpdate builds the seat-list payload.
func (t *Table) seatUpdate() seatUpdate {
	return seatUpdate{TableID: t.Cfg.ID, Seats: t.seatViews()}
}

// seatViews renders every seat's public state, reading live stacks from the
// hand when one is running so the view reflects committed chips.
func (t *Table) seatViews() []seatView {
	out := make([]seatView, 0, len(t.seats))
	for id, s := range t.seats {
		v := seatView{
			Seat: id, PlayerID: s.playerID, Stack: int64(s.stack),
			SittingOut: s.sittingOut, Disconnected: s.disconnected,
		}
		if t.Hand != nil {
			if i := t.handIndex(id); i >= 0 {
				v.Stack = int64(t.Hand.Players[i].Stack)
				v.InHand = t.Hand.Players[i].Status != engine.Folded
				v.Committed = int64(t.Hand.Players[i].Committed)
			}
		}
		out = append(out, v)
	}
	return out
}
