package engine

import "errors"

// HandConfig is the input needed to start a new hand. The deck is a pre-shuffled
// 52-card slice supplied by package fair; the engine adds no randomness of its
// own. Players are seats in physical seat order; ButtonPos indexes into it.
type HandConfig struct {
	Players    []Player // seats in order; Active seats are dealt in
	Deck       []Card   // already shuffled, len == NumCards
	ButtonPos  int      // index into Players of the dealer button
	SmallBlind Chips
	BigBlind   Chips
}

var (
	ErrBadDeck    = errors.New("engine.NewHand: deck must contain 52 cards")
	ErrBadButton  = errors.New("engine.NewHand: button position out of range")
	ErrTooFewPlay = errors.New("engine.NewHand: need at least two active players")
)

// NewHand deals hole cards, posts blinds, and returns a HandState ready for the
// first preflop action. It is pure: the same config always yields the same
// state. Dealing is round-robin one card at a time starting left of the button,
// matching live poker. Blinds are posted with the same commit logic as betting,
// so a short stack posts all-in.
func NewHand(cfg HandConfig) (HandState, error) {
	if len(cfg.Deck) != NumCards {
		return HandState{}, ErrBadDeck
	}
	n := len(cfg.Players)
	if cfg.ButtonPos < 0 || cfg.ButtonPos >= n {
		return HandState{}, ErrBadButton
	}

	h := HandState{
		Players:    append([]Player(nil), cfg.Players...),
		Board:      nil,
		Deck:       cfg.Deck,
		Street:     Preflop,
		ButtonPos:  cfg.ButtonPos,
		SmallBlind: cfg.SmallBlind,
		BigBlind:   cfg.BigBlind,
		acted:      make([]bool, n),
	}

	inHand := h.activeSeats()
	if len(inHand) < 2 {
		return HandState{}, ErrTooFewPlay
	}

	// Deal two hole cards, one at a time, round-robin starting left of button.
	for round := 0; round < 2; round++ {
		for off := 1; off <= n; off++ {
			i := (cfg.ButtonPos + off) % n
			if h.Players[i].Status != Active {
				continue
			}
			h.Players[i].Hole[round] = h.Deck[h.deckPos]
			h.deckPos++
		}
	}

	sbPos, bbPos, firstToAct := h.blindPositions(inHand)

	// Post blinds using the shared commit logic (short stacks go all-in).
	h.CurrentBet = 0
	h.commit(sbPos, min(h.SmallBlind, h.Players[sbPos].Stack))
	h.commit(bbPos, min(h.BigBlind, h.Players[bbPos].Stack))

	// The bet to match is the full big blind even if the BB posted short.
	h.CurrentBet = h.BigBlind
	h.MinRaise = h.BigBlind
	h.ToActPos = firstToAct
	// Blind posts are forced, not voluntary actions: leave acted flags false so
	// the big blind retains the option to check or raise on a limped pot.

	return h, nil
}

// activeSeats returns the indices of seats dealt into the hand, in seat order.
func (h *HandState) activeSeats() []int {
	var out []int
	for i := range h.Players {
		if h.Players[i].Status == Active {
			out = append(out, i)
		}
	}
	return out
}

// blindPositions returns the small-blind seat, big-blind seat, and the first
// seat to act preflop. Heads-up is the special case: the button posts the small
// blind and acts first preflop; the other seat posts the big blind.
func (h *HandState) blindPositions(inHand []int) (sbPos, bbPos, firstToAct int) {
	if len(inHand) == 2 {
		// Heads-up: button is the small blind and acts first preflop.
		sbPos = h.ButtonPos
		bbPos = h.nextActor(h.ButtonPos)
		return sbPos, bbPos, sbPos
	}
	sbPos = h.nextActor(h.ButtonPos)
	bbPos = h.nextActor(sbPos)
	firstToAct = h.nextActor(bbPos)
	return sbPos, bbPos, firstToAct
}
