package engine

import "errors"

// Chips is an integer count. NEVER use floats in money paths (see CLAUDE.md).
type Chips int64

// Street is the current betting round.
type Street uint8

const (
	Preflop Street = iota
	Flop
	Turn
	River
	Showdown
)

// PlayerStatus tracks a seat within a hand.
type PlayerStatus uint8

const (
	Active   PlayerStatus = iota // still in the hand, can act
	Folded                       // out of this hand
	AllIn                        // committed all chips, cannot act further
	SittingOut                   // not dealt in
)

// Player is one seat's per-hand state. The engine copies, never mutates in place.
type Player struct {
	SeatID    int
	Stack     Chips
	Committed Chips // chips put in this street
	TotalBet  Chips // chips put in this hand (for side-pot math)
	Status    PlayerStatus
	Hole      [2]Card
}

// HandState is the full authoritative state of one hand. Treat as immutable:
// Apply returns a new HandState.
type HandState struct {
	Players   []Player
	Board     []Card
	Deck      []Card // remaining undealt cards (shuffled input from package fair)
	deckPos   int
	Street    Street
	ButtonPos int
	ToActPos  int
	CurrentBet Chips // highest committed this street
	MinRaise  Chips
	Pot       Chips
	SmallBlind Chips
	BigBlind  Chips
}

// ActionKind enumerates legal player actions.
type ActionKind uint8

const (
	Fold ActionKind = iota
	Check
	Call
	Bet
	Raise
)

// Action is a player command applied to a HandState.
type Action struct {
	SeatID int
	Kind   ActionKind
	Amount Chips // target total for Bet/Raise (to-amount, not delta)
}

var (
	ErrNotYourTurn   = errors.New("not this seat's turn to act")
	ErrIllegalAction = errors.New("illegal action for current state")
	ErrBadAmount     = errors.New("bet/raise amount below minimum or above stack")
)

// Apply validates and applies one action, returning the next HandState.
// It never mutates the receiver. On error the input state is unchanged.
//
// NOTE: This is the core loop scaffold. Side-pot distribution at showdown lives
// in pots.go and is the most test-critical path in the codebase.
func (h HandState) Apply(a Action) (HandState, error) {
	if h.Street == Showdown {
		return h, ErrIllegalAction
	}
	next := h.clone()
	pi := next.seatIndex(a.SeatID)
	if pi < 0 || pi != next.ToActPos {
		return h, ErrNotYourTurn
	}
	p := &next.Players[pi]
	if p.Status != Active {
		return h, ErrIllegalAction
	}

	switch a.Kind {
	case Fold:
		p.Status = Folded
	case Check:
		if p.Committed != next.CurrentBet {
			return h, ErrIllegalAction
		}
	case Call:
		pay := min(next.CurrentBet-p.Committed, p.Stack)
		next.commit(pi, pay)
	case Bet, Raise:
		if a.Amount > p.Committed+p.Stack {
			return h, ErrBadAmount
		}
		if a.Amount < next.CurrentBet+next.MinRaise && a.Amount != p.Committed+p.Stack {
			return h, ErrBadAmount // must raise by at least min-raise unless all-in
		}
		next.MinRaise = a.Amount - next.CurrentBet
		next.commit(pi, a.Amount-p.Committed)
		next.CurrentBet = a.Amount
	default:
		return h, ErrIllegalAction
	}

	next.advance()
	return next, nil
}

func (h *HandState) commit(pi int, amount Chips) {
	p := &h.Players[pi]
	if amount > p.Stack {
		amount = p.Stack
	}
	p.Stack -= amount
	p.Committed += amount
	p.TotalBet += amount
	h.Pot += amount
	if p.Stack == 0 {
		p.Status = AllIn
	}
}

// advance moves action to the next actor, or the next street if the round closed.
func (h *HandState) advance() {
	if h.countActive() <= 1 {
		h.Street = Showdown
		return
	}
	next := h.nextActor(h.ToActPos)
	if next == -1 || h.roundClosed() {
		h.nextStreet()
		return
	}
	h.ToActPos = next
}

// roundClosed reports whether every active player has matched CurrentBet.
func (h *HandState) roundClosed() bool {
	for i := range h.Players {
		p := h.Players[i]
		if p.Status == Active && p.Committed != h.CurrentBet {
			return false
		}
	}
	return true
}

func (h *HandState) nextStreet() {
	for i := range h.Players {
		h.Players[i].Committed = 0
	}
	h.CurrentBet = 0
	h.MinRaise = h.BigBlind
	switch h.Street {
	case Preflop:
		h.Street = Flop
		h.dealBoard(3)
	case Flop:
		h.Street = Turn
		h.dealBoard(1)
	case Turn:
		h.Street = River
		h.dealBoard(1)
	case River:
		h.Street = Showdown
		return
	}
	h.ToActPos = h.nextActor(h.ButtonPos)
}

func (h *HandState) dealBoard(n int) {
	for i := 0; i < n; i++ {
		h.Board = append(h.Board, h.Deck[h.deckPos])
		h.deckPos++
	}
}

func (h *HandState) nextActor(from int) int {
	n := len(h.Players)
	for off := 1; off <= n; off++ {
		i := (from + off) % n
		if h.Players[i].Status == Active {
			return i
		}
	}
	return -1
}

func (h *HandState) countActive() int {
	c := 0
	for _, p := range h.Players {
		if p.Status == Active || p.Status == AllIn {
			c++
		}
	}
	return c
}

func (h HandState) seatIndex(seatID int) int {
	for i, p := range h.Players {
		if p.SeatID == seatID {
			return i
		}
	}
	return -1
}

func (h HandState) clone() HandState {
	c := h
	c.Players = append([]Player(nil), h.Players...)
	c.Board = append([]Card(nil), h.Board...)
	// Deck is treated read-only after deal; share the backing array.
	return c
}
