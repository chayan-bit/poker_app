package engine

import "sort"

// HandCategory ranks 5-card hand classes low-to-high.
type HandCategory uint8

const (
	HighCard HandCategory = iota
	Pair
	TwoPair
	Trips
	Straight
	Flush
	FullHouse
	Quads
	StraightFlush
)

var categoryNames = [...]string{
	"High Card", "Pair", "Two Pair", "Three of a Kind", "Straight",
	"Flush", "Full House", "Four of a Kind", "Straight Flush",
}

func (c HandCategory) String() string { return categoryNames[c] }

// HandValue is a totally-ordered hand strength. Compare with Beats/Compare.
// Category is primary; tiebreak holds kickers in descending significance.
type HandValue struct {
	Category HandCategory
	Tiebreak [5]Rank
}

// Compare returns -1, 0, +1 (a<b, a==b, a>b).
func (a HandValue) Compare(b HandValue) int {
	if a.Category != b.Category {
		if a.Category < b.Category {
			return -1
		}
		return 1
	}
	for i := 0; i < 5; i++ {
		if a.Tiebreak[i] != b.Tiebreak[i] {
			if a.Tiebreak[i] < b.Tiebreak[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Beats reports whether a is strictly stronger than b.
func (a HandValue) Beats(b HandValue) bool { return a.Compare(b) > 0 }

// Best5 evaluates the best 5-card hand from 5..7 cards (Texas Hold'em: 2 hole + board).
// It is O(C(n,5)) which is at most 21 combinations; fast and allocation-light.
func Best5(cards []Card) HandValue {
	if len(cards) < 5 {
		panic("engine.Best5: need at least 5 cards")
	}
	var best HandValue
	var first = true
	n := len(cards)
	var combo [5]Card
	// choose 5 of n
	idx := []int{0, 1, 2, 3, 4}
	for {
		for i, ci := range idx {
			combo[i] = cards[ci]
		}
		v := eval5(combo)
		if first || v.Beats(best) {
			best, first = v, false
		}
		// advance combination
		i := 4
		for i >= 0 && idx[i] == n-5+i {
			i--
		}
		if i < 0 {
			break
		}
		idx[i]++
		for j := i + 1; j < 5; j++ {
			idx[j] = idx[j-1] + 1
		}
	}
	return best
}

func eval5(c [5]Card) HandValue {
	ranks := []Rank{c[0].Rank, c[1].Rank, c[2].Rank, c[3].Rank, c[4].Rank}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i] > ranks[j] })

	flush := c[0].Suit == c[1].Suit && c[1].Suit == c[2].Suit &&
		c[2].Suit == c[3].Suit && c[3].Suit == c[4].Suit

	high, straight := straightHigh(ranks)

	// count rank multiplicities
	counts := map[Rank]int{}
	for _, r := range ranks {
		counts[r]++
	}
	type rc struct {
		rank  Rank
		count int
	}
	groups := make([]rc, 0, 5)
	for r, n := range counts {
		groups = append(groups, rc{r, n})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].count != groups[j].count {
			return groups[i].count > groups[j].count
		}
		return groups[i].rank > groups[j].rank
	})

	tb := func(rs ...Rank) [5]Rank {
		var out [5]Rank
		copy(out[:], rs)
		return out
	}

	switch {
	case straight && flush:
		return HandValue{StraightFlush, tb(high)}
	case groups[0].count == 4:
		return HandValue{Quads, tb(groups[0].rank, groups[1].rank)}
	case groups[0].count == 3 && groups[1].count == 2:
		return HandValue{FullHouse, tb(groups[0].rank, groups[1].rank)}
	case flush:
		return HandValue{Flush, tb(ranks...)}
	case straight:
		return HandValue{Straight, tb(high)}
	case groups[0].count == 3:
		return HandValue{Trips, tb(groups[0].rank, groups[1].rank, groups[2].rank)}
	case groups[0].count == 2 && groups[1].count == 2:
		return HandValue{TwoPair, tb(groups[0].rank, groups[1].rank, groups[2].rank)}
	case groups[0].count == 2:
		return HandValue{Pair, tb(groups[0].rank, groups[1].rank, groups[2].rank, groups[3].rank)}
	default:
		return HandValue{HighCard, tb(ranks...)}
	}
}

// straightHigh returns the high card of a straight (handling wheel A-2-3-4-5),
// and whether the 5 distinct ranks form a straight.
func straightHigh(desc []Rank) (Rank, bool) {
	// desc is sorted descending; require 5 distinct consecutive.
	for i := 1; i < 5; i++ {
		if desc[i] == desc[i-1] {
			return 0, false // a pair means not a straight
		}
	}
	if desc[0]-desc[4] == 4 {
		return desc[0], true
	}
	// wheel: A,5,4,3,2
	if desc[0] == RankAce && desc[1] == 5 && desc[2] == 4 && desc[3] == 3 && desc[4] == 2 {
		return 5, true
	}
	return 0, false
}
