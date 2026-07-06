package localcore

import "github.com/chayan-bit/poker_app/server/internal/engine"

// The payload shapes below mirror internal/table's outbound Data structs
// byte-for-byte (same JSON field names and order) so a client renders offline
// and online hands identically. They are redeclared here rather than imported
// because the table package keeps them unexported.

// cmdSitDown is the decoded body of a sit_down command.
type cmdSitDown struct {
	TableID string `json:"tableId"`
	Seat    int    `json:"seat"`
	BuyIn   int64  `json:"buyIn"`
}

// cmdRebuy is the decoded body of a rebuy command.
type cmdRebuy struct {
	TableID string `json:"tableId"`
	Amount  int64  `json:"amount"`
}

// cmdTimeout is the decoded body of the explicit fold-on-timeout command. It
// carries the coordinator's timestamp so the transition is log-driven and never
// derived from a local clock (issue #27 distributed-design scope).
type cmdTimeout struct {
	TableID string `json:"tableId"`
	NowMs   int64  `json:"nowMs"`
}

// seatView is one seat's public state (no hole cards).
type seatView struct {
	Seat         int    `json:"seat"`
	PlayerID     string `json:"playerId"`
	Stack        int64  `json:"stack"`
	SittingOut   bool   `json:"sittingOut"`
	InHand       bool   `json:"inHand"`
	Committed    int64  `json:"committed"`
	Disconnected bool   `json:"disconnected"`
}

// seatUpdate is broadcast whenever the set of seats or their stacks change.
type seatUpdate struct {
	TableID string     `json:"tableId"`
	Seats   []seatView `json:"seats"`
}

// tableSnapshot is the full public view sent to a joiner or on resync.
type tableSnapshot struct {
	TableID     string     `json:"tableId"`
	Seats       []seatView `json:"seats"`
	Button      int        `json:"button"`
	HandRunning bool       `json:"handRunning"`
	HandID      string     `json:"handId"`
	Street      string     `json:"street"`
	Board       []string   `json:"board"`
	Pot         int64      `json:"pot"`
	ToAct       int        `json:"toAct"`
	CurrentBet  int64      `json:"currentBet"`
	YourSeat    int        `json:"yourSeat"`
	YourHole    []string   `json:"yourHole,omitempty"`
}

// betPlaced is broadcast after a validated betting action.
type betPlaced struct {
	Seat       int    `json:"seat"`
	Kind       string `json:"kind"`
	Amount     int64  `json:"amount"`
	Pot        int64  `json:"pot"`
	ToAct      int    `json:"toAct"`
	CurrentBet int64  `json:"currentBet"`
	ToCall     int64  `json:"toCall"`
}

// tableStatus tells clients whether the room waits for the host to deal.
type tableStatus struct {
	TableID        string `json:"tableId"`
	WaitingForHost bool   `json:"waitingForHost"`
	SeatedCount    int    `json:"seatedCount"`
}

// streetAdvanced is broadcast when the betting round moves to a new street.
type streetAdvanced struct {
	Street string   `json:"street"`
	Board  []string `json:"board"`
}

// showdown is broadcast at the end of a hand, revealing every non-folded seat.
type showdown struct {
	HandID   string           `json:"handId"`
	Board    []string         `json:"board"`
	Results  map[int]string   `json:"results"`
	Awards   []engine.Award   `json:"awards"`
	Revealed map[int][]string `json:"revealed"`
}

// streetName renders an engine.Street for the wire.
func streetName(s engine.Street) string {
	switch s {
	case engine.Preflop:
		return "preflop"
	case engine.Flop:
		return "flop"
	case engine.Turn:
		return "turn"
	case engine.River:
		return "river"
	case engine.Showdown:
		return "showdown"
	default:
		return "unknown"
	}
}

// cardsToStrings renders cards as e.g. ["As","Kd"].
func cardsToStrings(cards []engine.Card) []string {
	out := make([]string, len(cards))
	for i, c := range cards {
		out[i] = c.String()
	}
	return out
}

// currentBet is the highest committed on the current street, or 0 with no hand.
func (lt *LocalTable) currentBet() int64 {
	if lt.hand == nil || lt.hand.Street == engine.Showdown {
		return 0
	}
	return int64(lt.hand.CurrentBet)
}

// toCall is the chips the seat now to act must add to call, floored at 0.
func (lt *LocalTable) toCall() int64 {
	if lt.hand == nil || lt.hand.Street == engine.Showdown {
		return 0
	}
	p := lt.hand.Players[lt.hand.ToActPos]
	d := int64(lt.hand.CurrentBet - p.Committed)
	if d < 0 {
		d = 0
	}
	return d
}

// toActSeat returns the seat currently to act, or -1 when no hand is running.
func (lt *LocalTable) toActSeat() int {
	if lt.hand == nil || lt.hand.Street == engine.Showdown {
		return -1
	}
	return lt.hand.Players[lt.hand.ToActPos].SeatID
}

// seatViews renders every seat's public state in ascending seat order, reading
// live stacks from the hand when one is running.
func (lt *LocalTable) seatViews() []seatView {
	ids := lt.sortedSeatIDs()
	views := make([]seatView, 0, len(ids))
	for _, id := range ids {
		s := lt.seats[id]
		v := seatView{
			Seat: id, PlayerID: s.playerID, Stack: int64(s.stack),
			SittingOut: s.sittingOut, Disconnected: s.disconnected,
		}
		if lt.hand != nil {
			if i := lt.handIndex(id); i >= 0 {
				v.Stack = int64(lt.hand.Players[i].Stack)
				v.InHand = lt.hand.Players[i].Status != engine.Folded
				v.Committed = int64(lt.hand.Players[i].Committed)
			}
		}
		views = append(views, v)
	}
	return views
}

// seatUpdate builds the seat-list payload.
func (lt *LocalTable) seatUpdatePayload() seatUpdate {
	return seatUpdate{TableID: lt.cfg.ID, Seats: lt.seatViews()}
}

// snapshot builds the full public table view (no players' hole cards).
func (lt *LocalTable) snapshot() tableSnapshot {
	snap := tableSnapshot{
		TableID:    lt.cfg.ID,
		Seats:      lt.seatViews(),
		Button:     lt.button,
		ToAct:      lt.toActSeat(),
		Street:     "none",
		CurrentBet: lt.currentBet(),
		YourSeat:   -1,
	}
	if lt.hand != nil {
		snap.HandRunning = true
		snap.HandID = lt.handID
		snap.Street = streetName(lt.hand.Street)
		snap.Board = cardsToStrings(lt.hand.Board)
		snap.Pot = int64(lt.hand.Pot)
	}
	return snap
}

// snapshotFor personalizes snapshot with the recipient's own hole cards when a
// hand is running and they are dealt in. It only ever reveals the recipient's
// own cards.
func (lt *LocalTable) snapshotFor(playerID string) tableSnapshot {
	snap := lt.snapshot()
	seat, ok := lt.seatOf(playerID)
	if !ok || lt.hand == nil {
		return snap
	}
	i := lt.handIndex(seat)
	if i < 0 {
		return snap
	}
	p := lt.hand.Players[i]
	if p.Status == engine.Folded {
		snap.YourSeat = seat
		return snap
	}
	snap.YourSeat = seat
	snap.YourHole = []string{p.Hole[0].String(), p.Hole[1].String()}
	return snap
}
