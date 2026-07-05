package engine

import "sort"

// Award is the result of showdown distribution for one seat.
type Award struct {
	SeatID int
	Amount Chips
}

// Pot is a (main or side) pot with the seats eligible to win it.
type Pot struct {
	Amount   Chips
	Eligible []int // seat IDs
}

// BuildPots computes main and side pots from each player's TotalBet.
// This is the classic layered side-pot algorithm: peel off the smallest
// all-in contribution as a layer, everyone who put in at least that much
// contributes one unit, and eligibility is limited to non-folded seats.
//
// This function and Distribute below are the highest-value test targets in the
// engine; split pots and multi-way all-ins are where poker apps silently rot.
func BuildPots(players []Player) []Pot {
	type contrib struct {
		seatID int
		amt    Chips
		folded bool
	}
	cs := make([]contrib, 0, len(players))
	for _, p := range players {
		if p.TotalBet > 0 {
			cs = append(cs, contrib{p.SeatID, p.TotalBet, p.Status == Folded})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].amt < cs[j].amt })

	var pots []Pot
	var prev Chips
	for i := range cs {
		layer := cs[i].amt - prev
		if layer <= 0 {
			continue
		}
		var amount Chips
		var eligible []int
		for j := i; j < len(cs); j++ {
			amount += layer
			if !cs[j].folded {
				eligible = append(eligible, cs[j].seatID)
			}
		}
		if amount > 0 && len(eligible) > 0 {
			pots = append(pots, Pot{Amount: amount, Eligible: eligible})
		}
		prev = cs[i].amt
	}
	return pots
}

// Distribute settles all pots given each seat's best hand value. Odd chips go
// to the earliest eligible seat by position (dealer-left convention should be
// applied by the caller ordering Eligible; here we use seat order as given).
func Distribute(pots []Pot, best map[int]HandValue) []Award {
	awards := map[int]Chips{}
	for _, pot := range pots {
		winners := bestSeats(pot.Eligible, best)
		if len(winners) == 0 {
			continue
		}
		share := pot.Amount / Chips(len(winners))
		remainder := pot.Amount - share*Chips(len(winners))
		for i, s := range winners {
			amt := share
			if Chips(i) < remainder {
				amt++ // distribute odd chips one each to earliest winners
			}
			awards[s] += amt
		}
	}
	out := make([]Award, 0, len(awards))
	for s, a := range awards {
		out = append(out, Award{SeatID: s, Amount: a})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SeatID < out[j].SeatID })
	return out
}

func bestSeats(eligible []int, best map[int]HandValue) []int {
	var winners []int
	var top HandValue
	first := true
	for _, s := range eligible {
		v, ok := best[s]
		if !ok {
			continue
		}
		if first || v.Beats(top) {
			top, winners, first = v, []int{s}, false
		} else if v.Compare(top) == 0 {
			winners = append(winners, s)
		}
	}
	return winners
}
