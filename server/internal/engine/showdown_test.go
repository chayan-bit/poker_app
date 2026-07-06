package engine

import "testing"

// hole builds a [2]Card from two card strings.
func hole(a, b string) [2]Card { return [2]Card{c(a), c(b)} }

// awardMap flattens awards into seatID -> amount for easy assertions.
func awardMap(aws []Award) map[int]Chips {
	m := map[int]Chips{}
	for _, a := range aws {
		m[a.SeatID] = a.Amount
	}
	return m
}

func TestSettle_NotShowdown(t *testing.T) {
	h := HandState{Street: River}
	if _, err := Settle(h); err != ErrNotShowdown {
		t.Fatalf("err = %v, want ErrNotShowdown", err)
	}
}

func TestSettle_Uncontested(t *testing.T) {
	h := HandState{
		Street: Showdown,
		Pot:    150,
		Players: []Player{
			{SeatID: 0, Stack: 100, Status: Active, TotalBet: 50},
			{SeatID: 1, Stack: 0, Status: Folded, TotalBet: 50},
			{SeatID: 2, Stack: 0, Status: Folded, TotalBet: 50},
		},
	}
	got, err := Settle(h)
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if len(got.Awards) != 1 || got.Awards[0].SeatID != 0 || got.Awards[0].Amount != 150 {
		t.Fatalf("awards = %+v, want seat 0 wins 150", got.Awards)
	}
	if got.Results != nil {
		t.Errorf("Results should be nil when uncontested")
	}
	if got.Players[0].Stack != 250 {
		t.Errorf("winner stack = %d, want 250 (100+150)", got.Players[0].Stack)
	}
	// Input must be untouched.
	if h.Players[0].Stack != 100 {
		t.Errorf("Settle mutated input players")
	}
}

func TestSettle_ThreeWayTwoSidePots_ShortStackWinsMainOnly(t *testing.T) {
	// Board: A K 7 2 9 rainbow-ish.
	board := cards("Ah", "Kd", "7c", "2s", "9h")
	h := HandState{
		Street:    Showdown,
		ButtonPos: 0,
		Board:     board,
		Pot:       700,
		Players: []Player{
			// seat0: trips aces (best), all-in for 100 -> eligible main pot only.
			{SeatID: 0, Stack: 0, Status: AllIn, TotalBet: 100, Hole: hole("Ac", "Ad")},
			// seat1: trips kings, TotalBet 300 -> wins the side pot.
			{SeatID: 1, Stack: 0, Status: AllIn, TotalBet: 300, Hole: hole("Kh", "Ks")},
			// seat2: pair of twos, TotalBet 300 -> loses both.
			{SeatID: 2, Stack: 0, Status: AllIn, TotalBet: 300, Hole: hole("2c", "3d")},
		},
	}
	got, err := Settle(h)
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	m := awardMap(got.Awards)
	// Main pot = 100*3 = 300 -> seat0 (trips aces). Side pot = 200*2 = 400 -> seat1.
	if m[0] != 300 {
		t.Errorf("seat0 award = %d, want 300 (main pot only)", m[0])
	}
	if m[1] != 400 {
		t.Errorf("seat1 award = %d, want 400 (side pot)", m[1])
	}
	if m[2] != 0 {
		t.Errorf("seat2 award = %d, want 0", m[2])
	}
	if got.Results[0].Category != Trips {
		t.Errorf("seat0 category = %v, want Trips", got.Results[0].Category)
	}
}

func TestSettle_SplitPotOddChipToEarliestLeftOfButton(t *testing.T) {
	// Broadway straight on the board: seats 0 and 1 both play the board and tie.
	// seat2 folded but its chip stays in the pot. Total pot = 50+50+1 = 101.
	board := cards("Ts", "Jh", "Qd", "Kc", "Ad")
	h := HandState{
		Street:    Showdown,
		ButtonPos: 0, // left of button is seat1 -> earliest for odd chip
		Board:     board,
		Pot:       101,
		Players: []Player{
			{SeatID: 0, Stack: 0, Status: Active, TotalBet: 50, Hole: hole("2c", "3d")},
			{SeatID: 1, Stack: 0, Status: Active, TotalBet: 50, Hole: hole("2h", "4s")},
			{SeatID: 2, Stack: 0, Status: Folded, TotalBet: 1, Hole: hole("5c", "6c")},
		},
	}
	got, err := Settle(h)
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	m := awardMap(got.Awards)
	if m[0]+m[1] != 101 {
		t.Fatalf("total distributed = %d, want 101 (folded chip stays in pot)", m[0]+m[1])
	}
	// Odd chip -> seat1 (left of button). seat1 = 51, seat0 = 50.
	if m[1] != 51 || m[0] != 50 {
		t.Errorf("split = seat0:%d seat1:%d, want 50/51 (odd chip to seat1)", m[0], m[1])
	}
	if _, ok := m[2]; ok {
		t.Errorf("folded seat2 should receive nothing")
	}
}

func TestSettle_OddChipFlipsWithButton(t *testing.T) {
	// Same tie, but move the button so seat0 is now left of the button.
	board := cards("Ts", "Jh", "Qd", "Kc", "Ad")
	h := HandState{
		Street:    Showdown,
		ButtonPos: 2, // left of button (seat index 2) is seat0
		Board:     board,
		Pot:       101,
		Players: []Player{
			{SeatID: 0, Stack: 0, Status: Active, TotalBet: 50, Hole: hole("2c", "3d")},
			{SeatID: 1, Stack: 0, Status: Active, TotalBet: 50, Hole: hole("2h", "4s")},
			{SeatID: 2, Stack: 0, Status: Folded, TotalBet: 1, Hole: hole("5c", "6c")},
		},
	}
	got, _ := Settle(h)
	m := awardMap(got.Awards)
	if m[0] != 51 || m[1] != 50 {
		t.Errorf("split = seat0:%d seat1:%d, want 51/50 (odd chip now to seat0)", m[0], m[1])
	}
}

func TestSettle_FoldedMoneyStaysInPot(t *testing.T) {
	// Heads-up-ish: seat0 wins, seat1 folded after committing. seat1's chips are
	// part of the pot and awarded to seat0.
	board := cards("Ah", "Kd", "7c", "2s", "9h")
	h := HandState{
		Street:    Showdown,
		ButtonPos: 0,
		Board:     board,
		Pot:       120,
		Players: []Player{
			{SeatID: 0, Stack: 0, Status: Active, TotalBet: 80, Hole: hole("Ac", "Ad")},
			{SeatID: 1, Stack: 0, Status: Folded, TotalBet: 40, Hole: hole("Kh", "Ks")},
		},
	}
	// One non-folded player -> uncontested, wins whole pot including folded chips.
	got, _ := Settle(h)
	m := awardMap(got.Awards)
	if m[0] != 120 {
		t.Errorf("seat0 award = %d, want 120 (includes folded seat1's 40)", m[0])
	}
}
