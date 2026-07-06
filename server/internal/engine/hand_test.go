package engine

import "testing"

// seatOf returns a fresh Active player with the given seat id and stack.
func seatOf(id int, stack Chips) Player {
	return Player{SeatID: id, Stack: stack, Status: Active}
}

// fullDeck returns an ordered 52-card deck (contents are irrelevant to lifecycle
// tests; only counts and positions matter, and evaluation is tested elsewhere).
func fullDeck() []Card { return OrderedDeck() }

// act applies an action and fails the test on error.
func act(t *testing.T, h HandState, a Action) HandState {
	t.Helper()
	next, err := h.Apply(a)
	if err != nil {
		t.Fatalf("Apply(%+v) unexpected error: %v", a, err)
	}
	return next
}

func TestNewHand_ThreeHandedBlindsAndOrder(t *testing.T) {
	players := []Player{seatOf(0, 1000), seatOf(1, 1000), seatOf(2, 1000)}
	h, err := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	if err != nil {
		t.Fatalf("NewHand: %v", err)
	}
	// Button seat 0 -> SB seat 1, BB seat 2, first to act seat 0 (UTG = button in 3-handed).
	if h.Players[1].Committed != 10 {
		t.Errorf("SB committed = %d, want 10", h.Players[1].Committed)
	}
	if h.Players[2].Committed != 20 {
		t.Errorf("BB committed = %d, want 20", h.Players[2].Committed)
	}
	if h.CurrentBet != 20 || h.MinRaise != 20 {
		t.Errorf("CurrentBet=%d MinRaise=%d, want 20/20", h.CurrentBet, h.MinRaise)
	}
	if h.Pot != 30 {
		t.Errorf("Pot = %d, want 30", h.Pot)
	}
	if h.ToActPos != 0 {
		t.Errorf("first to act = seat index %d, want 0", h.ToActPos)
	}
	// Each active player holds two hole cards.
	for i, p := range h.Players {
		if p.Hole[0] == (Card{}) || p.Hole[1] == (Card{}) {
			t.Errorf("seat %d not dealt two cards", i)
		}
	}
}

func TestNewHand_HeadsUpOrderPreflopAndPostflop(t *testing.T) {
	players := []Player{seatOf(0, 1000), seatOf(1, 1000)}
	h, err := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	if err != nil {
		t.Fatalf("NewHand: %v", err)
	}
	// Heads-up: button (seat 0) posts SB and acts first preflop.
	if h.Players[0].Committed != 10 {
		t.Errorf("button SB = %d, want 10", h.Players[0].Committed)
	}
	if h.Players[1].Committed != 20 {
		t.Errorf("BB = %d, want 20", h.Players[1].Committed)
	}
	if h.ToActPos != 0 {
		t.Errorf("preflop first to act = %d, want 0 (button)", h.ToActPos)
	}
	// Button calls, BB checks option -> flop. Postflop BB (seat 1) acts first.
	h = act(t, h, Action{SeatID: 0, Kind: Call})
	h = act(t, h, Action{SeatID: 1, Kind: Check})
	if h.Street != Flop {
		t.Fatalf("street = %v, want Flop", h.Street)
	}
	if h.ToActPos != 1 {
		t.Errorf("postflop first to act = %d, want 1 (BB, non-button)", h.ToActPos)
	}
}

func TestBigBlindOptionOnLimpedPot(t *testing.T) {
	players := []Player{seatOf(0, 1000), seatOf(1, 1000), seatOf(2, 1000)}
	h, _ := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	h = act(t, h, Action{SeatID: 0, Kind: Call}) // UTG/button limps
	h = act(t, h, Action{SeatID: 1, Kind: Call}) // SB completes
	// Everyone has matched the BB, but the round must NOT be closed: BB still
	// has the option. It should be BB's turn, still preflop.
	if h.Street != Preflop {
		t.Fatalf("street = %v, want Preflop (BB has option)", h.Street)
	}
	if h.ToActPos != 2 {
		t.Fatalf("to act = %d, want 2 (BB option)", h.ToActPos)
	}
	// BB checks its option -> advance to flop.
	h = act(t, h, Action{SeatID: 2, Kind: Check})
	if h.Street != Flop {
		t.Fatalf("street = %v, want Flop after BB checks option", h.Street)
	}
}

func TestBigBlindRaisesOption(t *testing.T) {
	players := []Player{seatOf(0, 1000), seatOf(1, 1000), seatOf(2, 1000)}
	h, _ := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	h = act(t, h, Action{SeatID: 0, Kind: Call})
	h = act(t, h, Action{SeatID: 1, Kind: Call})
	h = act(t, h, Action{SeatID: 2, Kind: Raise, Amount: 60}) // BB raises its option
	if h.Street != Preflop || h.CurrentBet != 60 {
		t.Fatalf("after BB raise: street=%v currentBet=%d", h.Street, h.CurrentBet)
	}
	// Action reopens to the limpers.
	if h.ToActPos != 0 {
		t.Errorf("to act = %d, want 0 (action reopened)", h.ToActPos)
	}
}

func TestShortStackBlindAllIn(t *testing.T) {
	// BB seat has fewer chips than the big blind: posts all-in for what it has.
	players := []Player{seatOf(0, 1000), seatOf(1, 1000), seatOf(2, 15)}
	h, _ := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	if h.Players[2].Status != AllIn {
		t.Errorf("short BB status = %v, want AllIn", h.Players[2].Status)
	}
	if h.Players[2].Committed != 15 {
		t.Errorf("short BB committed = %d, want 15", h.Players[2].Committed)
	}
	// The bet to match is still the full big blind.
	if h.CurrentBet != 20 {
		t.Errorf("CurrentBet = %d, want 20", h.CurrentBet)
	}
}

func TestIncompleteRaiseDoesNotReopen(t *testing.T) {
	// Seat 3 (button) opens, seat 1 (SB) calls a full raise's worth by re-raising,
	// then a short all-in that is less than a full raise must NOT let the seat
	// that already acted re-raise again.
	players := []Player{seatOf(0, 1000), seatOf(1, 1000), seatOf(2, 55)}
	h, _ := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	// Preflop: seat0 (button/UTG) raises to 40 (full raise, +20).
	h = act(t, h, Action{SeatID: 0, Kind: Raise, Amount: 40})
	// seat1 (SB) calls 40.
	h = act(t, h, Action{SeatID: 1, Kind: Call})
	// seat2 (BB, 55 chips, already 20 in) shoves all-in to 55 -> +15 over 40,
	// which is less than MinRaise(20): an incomplete raise.
	h = act(t, h, Action{SeatID: 2, Kind: Raise, Amount: 55})
	if h.Players[2].Status != AllIn {
		t.Fatalf("seat2 should be all-in")
	}
	if h.MinRaise != 20 {
		t.Errorf("MinRaise = %d, want 20 (incomplete raise must not change it)", h.MinRaise)
	}
	// Action returns to seat0, which already acted: it may call but NOT re-raise.
	if h.ToActPos != 0 {
		t.Fatalf("to act = %d, want 0", h.ToActPos)
	}
	if _, err := h.Apply(Action{SeatID: 0, Kind: Raise, Amount: 80}); err == nil {
		t.Errorf("seat0 re-raise after incomplete all-in should be rejected")
	}
	// Calling is still allowed.
	h = act(t, h, Action{SeatID: 0, Kind: Call})
	if h.Players[0].Committed != 55 {
		t.Errorf("seat0 committed = %d, want 55 (called the all-in)", h.Players[0].Committed)
	}
}

func TestAllInRunOutDealsToRiver(t *testing.T) {
	// Heads-up, both commit all chips preflop -> board runs out to the river and
	// the street becomes Showdown with no further action.
	players := []Player{seatOf(0, 200), seatOf(1, 200)}
	h, _ := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	h = act(t, h, Action{SeatID: 0, Kind: Raise, Amount: 200}) // button shoves
	h = act(t, h, Action{SeatID: 1, Kind: Call})               // BB calls all-in
	if h.Street != Showdown {
		t.Fatalf("street = %v, want Showdown after all-in run-out", h.Street)
	}
	if len(h.Board) != 5 {
		t.Errorf("board = %d cards, want 5 (full run-out)", len(h.Board))
	}
}

func TestOneActivePlusOneAllInRunsOut(t *testing.T) {
	// One player all-in and one covering caller (still has chips) is contested:
	// it must NOT end as uncontested; it runs out to showdown.
	players := []Player{seatOf(0, 100), seatOf(1, 1000)}
	h, _ := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	h = act(t, h, Action{SeatID: 0, Kind: Raise, Amount: 100}) // button all-in (100)
	h = act(t, h, Action{SeatID: 1, Kind: Call})               // BB calls, still has chips
	if ok, _ := h.IsUncontested(); ok {
		t.Fatalf("hand must not be uncontested with one Active + one AllIn")
	}
	if h.Street != Showdown || len(h.Board) != 5 {
		t.Fatalf("expected run-out to showdown; street=%v board=%d", h.Street, len(h.Board))
	}
}

func TestUncontestedFoldEndsHand(t *testing.T) {
	players := []Player{seatOf(0, 1000), seatOf(1, 1000), seatOf(2, 1000)}
	h, _ := NewHand(HandConfig{
		Players: players, Deck: fullDeck(), ButtonPos: 0,
		SmallBlind: 10, BigBlind: 20,
	})
	h = act(t, h, Action{SeatID: 0, Kind: Fold})
	h = act(t, h, Action{SeatID: 1, Kind: Fold})
	if h.Street != Showdown {
		t.Fatalf("street = %v, want Showdown after everyone folds to BB", h.Street)
	}
	ok, winner := h.IsUncontested()
	if !ok || winner != 2 {
		t.Fatalf("IsUncontested = (%v, %d), want (true, 2)", ok, winner)
	}
}

func TestNewHand_Validation(t *testing.T) {
	tests := []struct {
		name string
		cfg  HandConfig
	}{
		{"short deck", HandConfig{Players: []Player{seatOf(0, 100), seatOf(1, 100)}, Deck: make([]Card, 51), ButtonPos: 0}},
		{"bad button", HandConfig{Players: []Player{seatOf(0, 100), seatOf(1, 100)}, Deck: fullDeck(), ButtonPos: 5}},
		{"one player", HandConfig{Players: []Player{seatOf(0, 100)}, Deck: fullDeck(), ButtonPos: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewHand(tt.cfg); err == nil {
				t.Errorf("expected error for %s", tt.name)
			}
		})
	}
}
