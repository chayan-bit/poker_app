package table

import (
	"encoding/json"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// This file defines the table package's outbound event payloads and the inbound
// sit_down command payload. The wire *type* strings live in package protocol
// (the shared source of truth); these are the Data shapes carried inside an
// Envelope for the events protocol declares but does not yet give a struct.

// cmdSitDown is the decoded body of a sit_down command.
type cmdSitDown struct {
	TableID string `json:"tableId"`
	Seat    int    `json:"seat"`
	BuyIn   int64  `json:"buyIn"`
}

// seatView is one seat's public state (no hole cards).
type seatView struct {
	Seat       int    `json:"seat"`
	PlayerID   string `json:"playerId"`
	Stack      int64  `json:"stack"`
	SittingOut bool   `json:"sittingOut"`
	InHand     bool   `json:"inHand"`
}

// seatUpdate is broadcast whenever the set of seats or their stacks change.
type seatUpdate struct {
	TableID string     `json:"tableId"`
	Seats   []seatView `json:"seats"`
}

// tableSnapshot is the full public view sent to a joiner or on resync. It never
// contains other players' hole cards.
type tableSnapshot struct {
	TableID     string     `json:"tableId"`
	Seats       []seatView `json:"seats"`
	Button      int        `json:"button"`
	HandRunning bool       `json:"handRunning"`
	HandID      string     `json:"handId"`
	Street      string     `json:"street"`
	Board       []string   `json:"board"`
	Pot         int64      `json:"pot"`
	ToAct       int        `json:"toAct"` // seat to act, or -1
}

// betPlaced is broadcast after a validated betting action.
type betPlaced struct {
	Seat   int    `json:"seat"`
	Kind   string `json:"kind"`
	Amount int64  `json:"amount"`
	Pot    int64  `json:"pot"`
	ToAct  int    `json:"toAct"` // next seat to act, or -1
}

// streetAdvanced is broadcast when the betting round moves to a new street.
type streetAdvanced struct {
	Street string   `json:"street"`
	Board  []string `json:"board"`
}

// showdown is broadcast at the end of a contested (or uncontested) hand. It
// reveals the hole cards of every non-folded seat, the results, and the awards.
type showdown struct {
	HandID   string           `json:"handId"`
	Board    []string         `json:"board"`
	Results  map[int]string   `json:"results"`
	Awards   []engine.Award   `json:"awards"`
	Revealed map[int][]string `json:"revealed"`
}

// mustJSON marshals an outbound payload; the payloads here are always
// serializable, so an error is a programming bug.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("table: outbound payload not serializable: " + err.Error())
	}
	return b
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
