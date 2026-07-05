// Package protocol defines the wire messages shared between client and server.
// Invariants (see CLAUDE.md):
//   - Server -> client: past-tense EVENTS (hand_dealt, bet_placed).
//   - Client -> server: imperative COMMANDS (place_bet, sit_down).
//   - Every message carries a version and a monotonic Seq for delta ordering.
//   - Keep every serialized message under 1 KB.
//
// This is the single source of truth; the client's TypeScript types must mirror
// these shapes exactly (codegen target, see docs/Design_suite.md).
package protocol

import "encoding/json"

// ProtocolVersion is bumped on any breaking wire change.
const ProtocolVersion = 1

// Envelope wraps every message. Type routes; Seq orders; Data is the payload.
type Envelope struct {
	V    int             `json:"v"`
	Type string          `json:"type"`
	Seq  uint64          `json:"seq,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// ---- Client -> server commands ----

const (
	CmdJoinTable = "join_table"
	CmdSitDown   = "sit_down"
	CmdPlaceBet  = "place_bet" // covers check/call/bet/raise via Action
	CmdFold      = "fold"
	CmdLeave     = "leave_table"
	CmdResync    = "resync" // request a full snapshot after a detected Seq gap
)

// PlaceBet is the imperative betting command. Kind mirrors engine.ActionKind.
type PlaceBet struct {
	TableID string `json:"tableId"`
	Kind    string `json:"kind"`   // "check" | "call" | "bet" | "raise" | "fold"
	Amount  int64  `json:"amount"` // to-amount for bet/raise, ignored otherwise
}

// ---- Server -> client events ----

const (
	EvHandDealt  = "hand_dealt"
	EvBetPlaced  = "bet_placed"
	EvStreet     = "street_advanced"
	EvShowdown   = "showdown"
	EvSnapshot   = "table_snapshot"
	EvSeatUpdate = "seat_update"
	EvError      = "error"
	EvFairReveal = "fair_reveal" // seed revealed after the hand, for verification
)

// HandDealt announces a new hand. Only the recipient's own hole cards are sent;
// opponents' cards are withheld until showdown (never trust the client).
type HandDealt struct {
	TableID    string   `json:"tableId"`
	HandID     string   `json:"handId"`
	Commitment string   `json:"commitment"` // SHA-256(seed), published pre-deal
	YourSeat   int      `json:"yourSeat"`
	YourHole   []string `json:"yourHole"` // e.g. ["As","Kd"]
	ButtonSeat int      `json:"buttonSeat"`
	Blinds     [2]int64 `json:"blinds"`
}

// FairReveal lets any client recompute Shuffle(seed) and verify the deal.
type FairReveal struct {
	HandID     string `json:"handId"`
	Commitment string `json:"commitment"`
	Seed       string `json:"seed"`
}

// ErrorEvent is a non-fatal, human-readable problem (never leaks internals).
type ErrorEvent struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
