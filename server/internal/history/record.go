// Package history builds, stores, and exports permanent hand records.
//
// A HandRecord is the durable, append-only account of one played hand: seats,
// stacks, every action, the board, showdown results, and the fairness
// commitment/seed pair (see docs/Design_suite.md section 7 and package fair).
// It is built incrementally during the hand by a Recorder and only becomes
// immutable once Finish is called.
package history

import (
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// SeatInfo captures one seat's starting state and hole cards for the record.
type SeatInfo struct {
	SeatID     int
	PlayerID   string
	StartStack engine.Chips
	Hole       []string // e.g. ["As","Kd"]; empty until shown or folded face-up
}

// Event is one recorded happening within a hand, in chronological order.
type Event struct {
	Street string
	SeatID int
	Kind   string
	Amount engine.Chips
}

// HandRecord is the complete, durable account of one played hand.
type HandRecord struct {
	HandID     string
	TableID    string
	StartedAt  time.Time
	ButtonSeat int
	Blinds     [2]engine.Chips
	Commitment string
	SeedHex    string
	Seats      []SeatInfo
	Events     []Event
	Board      []string
	Awards     []engine.Award
	Results    map[int]string // seatID -> result description, e.g. "won 300 with Two Pair"
}

// Recorder incrementally builds one HandRecord as a hand plays out.
//
// Concurrency: a Recorder is NOT safe for concurrent use. It is owned by a
// single table loop goroutine that calls its methods in event order; no
// internal locking is performed by design.
type Recorder struct {
	rec HandRecord
}

// NewRecorder starts recording a new hand from its static, pre-deal metadata.
func NewRecorder(handID, tableID string, startedAt time.Time, buttonSeat int, blinds [2]engine.Chips, commitment string, seats []SeatInfo) *Recorder {
	return &Recorder{
		rec: HandRecord{
			HandID:     handID,
			TableID:    tableID,
			StartedAt:  startedAt,
			ButtonSeat: buttonSeat,
			Blinds:     blinds,
			Commitment: commitment,
			Seats:      append([]SeatInfo(nil), seats...),
			Events:     nil,
			Board:      nil,
			Results:    map[int]string{},
		},
	}
}

// OnAction appends one player action event on the given street.
func (r *Recorder) OnAction(street string, seatID int, kind string, amount engine.Chips) {
	r.rec.Events = append(r.rec.Events, Event{
		Street: street,
		SeatID: seatID,
		Kind:   kind,
		Amount: amount,
	})
}

// OnStreet records the transition to a new street and any cards it dealt.
// boardCards holds only the cards newly revealed on this street (e.g. 3 for
// the flop); they are appended to the cumulative Board.
func (r *Recorder) OnStreet(street string, boardCards []string) {
	r.rec.Events = append(r.rec.Events, Event{Street: street, Kind: "street"})
	r.rec.Board = append(r.rec.Board, boardCards...)
}

// OnShowdown records the pot awards and each seat's human-readable result.
func (r *Recorder) OnShowdown(awards []engine.Award, results map[int]string) {
	r.rec.Awards = append([]engine.Award(nil), awards...)
	for seatID, desc := range results {
		r.rec.Results[seatID] = desc
	}
}

// OnReveal records the fairness seed reveal, verifiable against Commitment.
func (r *Recorder) OnReveal(seedHex string) {
	r.rec.SeedHex = seedHex
}

// Finish returns the completed, immutable HandRecord.
func (r *Recorder) Finish() HandRecord {
	return r.rec
}
