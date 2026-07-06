package table

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// TestTournamentTableAppliesDirective drives one real hand on a tournament table
// with a stub controller, verifying the additive seam: the table pre-seats the
// registered players at the starting stack, waits for both to connect before
// dealing, then applies the controller's directive after the hand - raising
// blinds, removing the eliminated seat, and publishing the final result while
// halting further deals. The stub ignores the (random) hand outcome, so the
// assertions are fully deterministic.
func TestTournamentTableAppliesDirective(t *testing.T) {
	directive := TourneyDirective{
		SmallBlind: 15, BigBlind: 30, Level: 2, BlindsChanged: true,
		Eliminations: []protocol.Elimination{{Seat: 1, PlayerID: "bob", Place: 2}},
		Done:         true,
		Result: &protocol.TourneyResult{Places: []protocol.TourneyPlace{
			{PlayerID: "alice", Place: 1, Prize: 300},
			{PlayerID: "bob", Place: 2, Prize: 0},
		}},
	}
	var calls int
	stub := func(standings []SeatResult) TourneyDirective {
		calls++
		if len(standings) != 2 {
			t.Errorf("controller got %d standings, want 2", len(standings))
		}
		return directive
	}

	cfg := Config{
		ID: "t1", Visibility: Private, MaxSeats: 6, SmallBlind: 10, BigBlind: 20,
		AutoStart: true,
		Tournament: &TourneyRules{
			StartingStack: 1500, NoRebuy: true,
			Seats: []TourneySeat{{Seat: 0, PlayerID: "alice"}, {Seat: 1, PlayerID: "bob"}},
		},
	}
	h := newHarnessCfg(t, cfg, Deps{OnHandComplete: stub})

	// Pre-seated but not yet connected: no hand deals until both join.
	h.join("alice")
	h.expectNone("alice", protocol.EvHandDealt, 50*time.Millisecond)
	h.join("bob")

	da := decodeHandDealt(t, h.waitFor("alice", protocol.EvHandDealt))
	if da.Blinds != [2]int64{10, 20} {
		t.Fatalf("first hand must open at level-1 blinds 10/20, got %v", da.Blinds)
	}
	// Pre-seated at the tournament starting stack, no ledger buy-in (stack +
	// chips already committed to the blind account for the full 1500).
	if sv := seatInSnapshot(h.snapshot("alice"), 0); sv.Stack+sv.Committed != 1500 {
		t.Fatalf("seat 0 stack+committed = %d, want tournament starting stack 1500", sv.Stack+sv.Committed)
	}

	// Play the hand to showdown; the stub directive is applied at settle.
	h.driveCallDownToShowdown(map[int]string{0: "alice", 1: "bob"}, "alice")

	bu := decodeBlindsUp(t, h.waitFor("alice", protocol.EvBlindsUp))
	if bu.Level != 2 || bu.SB != 15 || bu.BB != 30 {
		t.Fatalf("blinds_up = %+v, want level 2 15/30", bu)
	}
	el := decodeElimination(t, h.waitFor("alice", protocol.EvElimination))
	if el.Seat != 1 || el.PlayerID != "bob" || el.Place != 2 {
		t.Fatalf("elimination = %+v, want seat 1 bob place 2", el)
	}
	res := decodeTourneyResult(t, h.waitFor("alice", protocol.EvTourneyResult))
	if len(res.Places) != 2 || res.Places[0].PlayerID != "alice" || res.Places[0].Prize != 300 {
		t.Fatalf("tourney_result = %+v, want alice winning 300", res)
	}
	// The tourney_result receive establishes happens-before with the callback,
	// so reading calls here is race-free.
	if calls != 1 {
		t.Fatalf("controller should be invoked exactly once, got %d", calls)
	}

	// The eliminated seat is gone and, being done, no further hand deals.
	if _, ok := seatPresent(h.snapshot("alice"), 1); ok {
		t.Fatalf("eliminated seat 1 must be removed from the table")
	}
	h.expectNone("alice", protocol.EvHandDealt, 100*time.Millisecond)
}

// TestTournamentRebuyRejected confirms NoRebuy tournaments reject a rebuy.
func TestTournamentRebuyRejected(t *testing.T) {
	cfg := Config{
		ID: "t1", MaxSeats: 6, SmallBlind: 10, BigBlind: 20, AutoStart: true,
		Tournament: &TourneyRules{StartingStack: 1500, NoRebuy: true,
			Seats: []TourneySeat{{Seat: 0, PlayerID: "alice"}, {Seat: 1, PlayerID: "bob"}}},
	}
	h := newHarnessCfg(t, cfg, Deps{OnHandComplete: func([]SeatResult) TourneyDirective { return TourneyDirective{} }})
	h.rebuy("alice", 100)
	if ee := decodeError(t, h.waitFor("alice", protocol.EvError)); ee.Code != "no_rebuy" {
		t.Fatalf("rebuy error = %q, want no_rebuy", ee.Code)
	}
}

func decodeBlindsUp(t *testing.T, ev protocol.Envelope) protocol.BlindsUp {
	t.Helper()
	var b protocol.BlindsUp
	if err := json.Unmarshal(ev.Data, &b); err != nil {
		t.Fatalf("decode blinds_up: %v", err)
	}
	return b
}

func decodeElimination(t *testing.T, ev protocol.Envelope) protocol.Elimination {
	t.Helper()
	var e protocol.Elimination
	if err := json.Unmarshal(ev.Data, &e); err != nil {
		t.Fatalf("decode elimination: %v", err)
	}
	return e
}

func decodeTourneyResult(t *testing.T, ev protocol.Envelope) protocol.TourneyResult {
	t.Helper()
	var r protocol.TourneyResult
	if err := json.Unmarshal(ev.Data, &r); err != nil {
		t.Fatalf("decode tourney_result: %v", err)
	}
	return r
}

// seatPresent reports whether seat appears in the snapshot.
func seatPresent(snap tableSnapshot, seat int) (seatView, bool) {
	for _, s := range snap.Seats {
		if s.Seat == seat {
			return s, true
		}
	}
	return seatView{}, false
}

var _ = engine.Chips(0)
