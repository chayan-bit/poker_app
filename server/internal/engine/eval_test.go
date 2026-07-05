package engine

import "testing"

func c(s string) Card {
	rank := map[byte]Rank{'2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7,
		'8': 8, '9': 9, 'T': 10, 'J': 11, 'Q': 12, 'K': 13, 'A': 14}[s[0]]
	suit := map[byte]Suit{'c': Clubs, 'd': Diamonds, 'h': Hearts, 's': Spades}[s[1]]
	return Card{Rank: rank, Suit: suit}
}

func cards(ss ...string) []Card {
	out := make([]Card, len(ss))
	for i, s := range ss {
		out[i] = c(s)
	}
	return out
}

func TestBest5Categories(t *testing.T) {
	tests := []struct {
		name string
		hand []string
		want HandCategory
	}{
		{"royal flush", []string{"As", "Ks", "Qs", "Js", "Ts", "2c", "3d"}, StraightFlush},
		{"quads", []string{"9s", "9h", "9d", "9c", "Ks", "2c", "3d"}, Quads},
		{"full house", []string{"9s", "9h", "9d", "Kc", "Ks", "2c", "3d"}, FullHouse},
		{"flush", []string{"2s", "5s", "9s", "Js", "Ks", "3c", "4d"}, Flush},
		{"wheel straight", []string{"As", "2h", "3d", "4c", "5s", "Kc", "Qd"}, Straight},
		{"trips", []string{"7s", "7h", "7d", "Kc", "Qs", "2c", "3d"}, Trips},
		{"two pair", []string{"7s", "7h", "Kd", "Kc", "Qs", "2c", "3d"}, TwoPair},
		{"pair", []string{"7s", "7h", "2d", "Kc", "Qs", "9c", "3d"}, Pair},
		{"high card", []string{"7s", "9h", "2d", "Kc", "Qs", "4c", "3d"}, HighCard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Best5(cards(tt.hand...))
			if got.Category != tt.want {
				t.Fatalf("got %v, want %v", got.Category, tt.want)
			}
		})
	}
}

func TestFlushBeatsStraight(t *testing.T) {
	flush := Best5(cards("2s", "5s", "9s", "Js", "Ks", "3c", "4d"))
	straight := Best5(cards("6s", "7h", "8d", "9c", "Ts", "2c", "3d"))
	if !flush.Beats(straight) {
		t.Fatal("flush should beat straight")
	}
}

func TestKickerDecidesPair(t *testing.T) {
	a := Best5(cards("As", "Ah", "Kd", "9c", "2s", "7c", "4d")) // pair aces, K kicker
	b := Best5(cards("As", "Ah", "Qd", "9c", "2s", "7c", "4d")) // pair aces, Q kicker
	if !a.Beats(b) {
		t.Fatal("higher kicker should win")
	}
}
