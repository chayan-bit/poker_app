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

	// acted[i] reports whether Players[i] has voluntarily acted since the last
	// full bet/raise on the current street. Posting a blind is NOT acting, so
	// the big blind still gets the option to check or raise on a limped pot. A
	// full raise clears every flag (action reopens); an incomplete all-in raise
	// does not, which enforces the standard "no re-raise" rule for seats that
	// have already acted.
	acted []bool
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
		next.acted[pi] = true
	case Check:
		if p.Committed != next.CurrentBet {
			return h, ErrIllegalAction
		}
		next.acted[pi] = true
	case Call:
		pay := min(next.CurrentBet-p.Committed, p.Stack)
		next.commit(pi, pay)
		next.acted[pi] = true
	case Bet, Raise:
		toAmt := a.Amount
		if toAmt <= next.CurrentBet {
			return h, ErrBadAmount // a bet/raise must exceed the current bet
		}
		if toAmt > p.Committed+p.Stack {
			return h, ErrBadAmount
		}
		allIn := toAmt == p.Committed+p.Stack
		fullRaise := toAmt >= next.CurrentBet+next.MinRaise
		if !fullRaise && !allIn {
			return h, ErrBadAmount // must raise by at least MinRaise unless all-in
		}
		// Incomplete-raise rule: a seat that has already acted at the current
		// bet level may not re-raise off an all-in that did not reopen action.
		// A full raise clears the acted flags below, so this only bites when the
		// last increase to CurrentBet was a short all-in.
		if next.acted[pi] {
			return h, ErrIllegalAction
		}
		next.commit(pi, toAmt-p.Committed)
		if fullRaise {
			next.MinRaise = toAmt - next.CurrentBet
			for i := range next.acted {
				next.acted[i] = false
			}
		}
		next.CurrentBet = toAmt
		next.acted[pi] = true
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

// advance moves action to the next actor, or the next street once the current
// betting round is closed. When only one player remains unfolded the hand ends
// immediately (uncontested). When betting is closed but at most one player can
// still act, the remaining streets are dealt out to showdown (all-in run-out).
func (h *HandState) advance() {
	if h.countNonFolded() <= 1 {
		h.Street = Showdown
		return
	}
	if nxt := h.nextToAct(h.ToActPos); nxt != -1 {
		h.ToActPos = nxt
		return
	}
	h.nextStreet()
}

// nextToAct returns the next seat (after from, in seat order) that still owes an
// action this street: an Active seat that has either not matched CurrentBet or
// not yet acted since the last full raise (the big-blind option case). Returns
// -1 when the betting round is closed.
func (h *HandState) nextToAct(from int) int {
	n := len(h.Players)
	for off := 1; off <= n; off++ {
		i := (from + off) % n
		p := h.Players[i]
		if p.Status == Active && (p.Committed != h.CurrentBet || !h.acted[i]) {
			return i
		}
	}
	return -1
}

// nextStreet resets per-street state and deals the next community cards. If no
// more than one player can voluntarily act (everyone else is all-in), it runs
// the board out to the river and lands on Showdown.
func (h *HandState) nextStreet() {
	runOut := h.countCanAct() <= 1
	for {
		h.resetStreet()
		h.dealNextStreet()
		if h.Street == Showdown {
			return
		}
		if !runOut {
			h.ToActPos = h.nextActor(h.ButtonPos)
			return
		}
	}
}

// resetStreet clears per-street betting state.
func (h *HandState) resetStreet() {
	for i := range h.Players {
		h.Players[i].Committed = 0
		h.acted[i] = false
	}
	h.CurrentBet = 0
	h.MinRaise = h.BigBlind
}

// dealNextStreet advances the Street enum and deals the board cards it needs.
func (h *HandState) dealNextStreet() {
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
	}
}

// IsUncontested reports whether exactly one player remains unfolded; if so it
// returns that seat's ID (the sole winner). Note an all-in seat is still "in":
// one Active plus one AllIn is contested and must be run out to showdown.
func (h HandState) IsUncontested() (bool, int) {
	count := 0
	winner := -1
	for _, p := range h.Players {
		if p.Status == Active || p.Status == AllIn {
			count++
			winner = p.SeatID
		}
	}
	if count == 1 {
		return true, winner
	}
	return false, -1
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

// countNonFolded counts players still in the hand (Active or AllIn).
func (h *HandState) countNonFolded() int {
	c := 0
	for _, p := range h.Players {
		if p.Status == Active || p.Status == AllIn {
			c++
		}
	}
	return c
}

// countCanAct counts players who can still voluntarily act (Active with chips).
func (h *HandState) countCanAct() int {
	c := 0
	for _, p := range h.Players {
		if p.Status == Active {
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
	c.acted = append([]bool(nil), h.acted...)
	// Deck is treated read-only after deal; share the backing array.
	return c
}
