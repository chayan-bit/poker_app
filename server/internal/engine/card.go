// Package engine is the pure, mutation-free poker core.
// No I/O, no randomness, no clock. Every function takes inputs and returns
// new values. The shuffled deck is an INPUT, produced by package fair.
package engine

import "fmt"

// Rank is 2..14 (Ace high = 14).
type Rank uint8

// Suit is 0..3.
type Suit uint8

const (
	Clubs Suit = iota
	Diamonds
	Hearts
	Spades
)

const (
	RankTwo   Rank = 2
	RankAce   Rank = 14
	NumCards       = 52
)

// Card is a compact value type: rank in high bits, suit in low bits.
type Card struct {
	Rank Rank
	Suit Suit
}

var rankRunes = map[Rank]byte{
	2: '2', 3: '3', 4: '4', 5: '5', 6: '6', 7: '7', 8: '8',
	9: '9', 10: 'T', 11: 'J', 12: 'Q', 13: 'K', 14: 'A',
}
var suitRunes = [4]byte{'c', 'd', 'h', 's'}

// String renders a card as e.g. "As", "Td", "2c".
func (c Card) String() string {
	return string([]byte{rankRunes[c.Rank], suitRunes[c.Suit]})
}

// Index maps a card to a stable 0..51 slot: (rank-2)*4 + suit.
// The deck ordering used by the provably-fair shuffle relies on this.
func (c Card) Index() int {
	return int(c.Rank-2)*4 + int(c.Suit)
}

// CardFromIndex is the inverse of Index.
func CardFromIndex(i int) Card {
	return Card{Rank: Rank(i/4) + 2, Suit: Suit(i % 4)}
}

// OrderedDeck returns the canonical 52-card deck in index order.
// The fair shuffle permutes THIS slice; the ordering must never change.
func OrderedDeck() []Card {
	d := make([]Card, NumCards)
	for i := range d {
		d[i] = CardFromIndex(i)
	}
	return d
}

func (c Card) GoString() string { return fmt.Sprintf("Card(%s)", c) }
