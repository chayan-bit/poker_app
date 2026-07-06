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
	CmdResync    = "resync"     // request a full snapshot after a detected Seq gap
	CmdStartHand = "start_hand" // host-only: begin the first hand in a private room
	CmdRebuy     = "rebuy"      // add chips between hands
	CmdSitOut    = "sit_out"    // stop being dealt in
	CmdSitIn     = "sit_in"     // resume being dealt in
	// CmdDisconnected is an internal command the WS gateway submits to every
	// table a connection had joined once its socket closes. It is never sent by
	// a real client; it drives the disconnect-grace flow.
	CmdDisconnected = "disconnected"
)

// PlaceBet is the imperative betting command. Kind mirrors engine.ActionKind.
type PlaceBet struct {
	TableID string `json:"tableId"`
	Kind    string `json:"kind"`   // "check" | "call" | "bet" | "raise" | "fold"
	Amount  int64  `json:"amount"` // to-amount for bet/raise, ignored otherwise
}

// ---- Server -> client events ----

const (
	EvHandDealt   = "hand_dealt"
	EvBetPlaced   = "bet_placed"
	EvStreet      = "street_advanced"
	EvShowdown    = "showdown"
	EvSnapshot    = "table_snapshot"
	EvSeatUpdate  = "seat_update"
	EvError       = "error"
	EvFairReveal  = "fair_reveal"  // seed revealed after the hand, for verification
	EvTableStatus = "table_status" // waiting-for-host / seated-count, drives a start button
	// Tournament (sit-and-go) events. Emitted only by tables running in
	// tournament mode; the sequencing logic lives in internal/tourney.
	EvBlindsUp      = "blinds_up"       // a blind level elapsed; blinds raised at hand start
	EvElimination   = "elimination"     // a seat busted (0 chips) and was removed
	EvTourneyResult = "tourney_result"  // tournament over: final places and prizes
)

// BlindsUp announces that the blind clock advanced to a new level, applied at
// the start of the hand that follows (never mid-hand).
type BlindsUp struct {
	Level int   `json:"level"` // 1-based level number now in effect
	SB    int64 `json:"sb"`
	BB    int64 `json:"bb"`
}

// Elimination announces a seat busting out of a tournament, with the place it
// finished in (1 == winner; larger == busted earlier).
type Elimination struct {
	Seat     int    `json:"seat"`
	PlayerID string `json:"playerId"`
	Place    int    `json:"place"`
}

// TourneyPlace is one finisher's final standing and prize (in cash chips).
type TourneyPlace struct {
	PlayerID string `json:"playerId"`
	Place    int    `json:"place"`
	Prize    int64  `json:"prize"`
}

// TourneyResult is broadcast when a tournament completes: every finisher's
// place and prize, ordered best place first.
type TourneyResult struct {
	Places []TourneyPlace `json:"places"`
}

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
