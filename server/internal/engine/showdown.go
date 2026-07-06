package engine

import "errors"

// SettledHand is the terminal result of a hand: chip awards, each contesting
// seat's evaluated hand, and the post-settlement player slice (stacks updated).
type SettledHand struct {
	Awards  []Award            // per-seat winnings, sorted by SeatID
	Results map[int]HandValue  // seatID -> best 5-card hand (nil when uncontested)
	Players []Player           // new slice with awards applied to stacks
}

// ErrNotShowdown is returned when Settle is called before the hand reaches
// Showdown.
var ErrNotShowdown = errors.New("engine.Settle: hand is not at showdown")

// Settle distributes the pot at showdown and returns the settled hand. It never
// mutates its input. When the hand is uncontested the sole remaining player wins
// the whole pot without evaluation. Otherwise it evaluates each non-folded seat,
// builds the layered side pots, and distributes them. Odd chips go to the
// earliest eligible seat starting left of the button (live-poker convention).
func Settle(h HandState) (SettledHand, error) {
	if h.Street != Showdown {
		return SettledHand{}, ErrNotShowdown
	}

	if ok, winner := h.IsUncontested(); ok {
		awards := []Award{{SeatID: winner, Amount: h.Pot}}
		return SettledHand{
			Awards:  awards,
			Results: nil,
			Players: applyAwards(h.Players, awards),
		}, nil
	}

	results := make(map[int]HandValue)
	for _, p := range h.Players {
		if p.Status == Folded || p.Status == SittingOut {
			continue
		}
		cards := append(append([]Card(nil), h.Board...), p.Hole[0], p.Hole[1])
		results[p.SeatID] = Best5(cards)
	}

	pots := BuildPots(h.Players)
	order := h.seatOrderFromButton()
	for i := range pots {
		pots[i].Eligible = reorderByPosition(pots[i].Eligible, order)
	}

	awards := Distribute(pots, results)
	return SettledHand{
		Awards:  awards,
		Results: results,
		Players: applyAwards(h.Players, awards),
	}, nil
}

// seatOrderFromButton lists seat IDs in action order starting left of the
// button. Distribute pays odd chips to the earliest seat in Eligible order, so
// ordering eligibility this way puts odd chips on the earliest seat left of the
// button.
func (h HandState) seatOrderFromButton() []int {
	n := len(h.Players)
	out := make([]int, 0, n)
	for off := 1; off <= n; off++ {
		out = append(out, h.Players[(h.ButtonPos+off)%n].SeatID)
	}
	return out
}

// reorderByPosition returns eligible seat IDs sorted into the given positional
// order, preserving BuildPots' layering (membership) while fixing tie order.
func reorderByPosition(eligible, order []int) []int {
	set := make(map[int]bool, len(eligible))
	for _, s := range eligible {
		set[s] = true
	}
	out := make([]int, 0, len(eligible))
	for _, s := range order {
		if set[s] {
			out = append(out, s)
		}
	}
	return out
}

// applyAwards returns a new player slice with each award added to the winner's
// stack. The input slice is never mutated.
func applyAwards(players []Player, awards []Award) []Player {
	out := append([]Player(nil), players...)
	byID := make(map[int]Chips, len(awards))
	for _, a := range awards {
		byID[a.SeatID] += a.Amount
	}
	for i := range out {
		out[i].Stack += byID[out[i].SeatID]
	}
	return out
}
