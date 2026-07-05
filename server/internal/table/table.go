// Package table owns live table state and the per-table event loop. Each table
// runs in its own goroutine and is the single writer of its state, so the pure
// engine never needs locks. Clients talk to a table only via its command chan.
package table

import (
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// Visibility controls who may join.
type Visibility uint8

const (
	Public Visibility = iota
	Private
)

// Config is the host-chosen ruleset for a table (see Design_suite 6.2).
type Config struct {
	ID         string
	Visibility Visibility
	MaxSeats   int
	SmallBlind engine.Chips
	BigBlind   engine.Chips
	JoinCode   string // 6-char code for private rooms
	// Feature toggles (Design_suite 9): RunItTwice, BombPots, Straddles, etc.
}

// Command is an inbound client request routed to the table goroutine.
type Command struct {
	PlayerID string
	Msg      protocol.Envelope
	Reply    chan<- protocol.Envelope // per-connection outbound
}

// Table is the live state. Only its own goroutine touches Hand/seats.
type Table struct {
	Cfg    Config
	Hand   *engine.HandState
	seats  map[int]string // seatID -> playerID
	subs   map[string]chan<- protocol.Envelope
	inbox  chan Command
	seq    uint64
}

// New builds a table and starts its event loop.
func New(cfg Config) *Table {
	t := &Table{
		Cfg:   cfg,
		seats: map[int]string{},
		subs:  map[string]chan<- protocol.Envelope{},
		inbox: make(chan Command, 64),
	}
	go t.loop()
	return t
}

// Submit enqueues a command; non-blocking beyond the buffered inbox.
func (t *Table) Submit(c Command) { t.inbox <- c }

// loop is the single-threaded owner of table state.
func (t *Table) loop() {
	for cmd := range t.inbox {
		switch cmd.Msg.Type {
		case protocol.CmdJoinTable:
			t.subs[cmd.PlayerID] = cmd.Reply
			// TODO: seat assignment, buy-in from economy, snapshot send.
		case protocol.CmdPlaceBet:
			t.handleBet(cmd)
		case protocol.CmdResync:
			// TODO: send full EvSnapshot to cmd.Reply.
		case protocol.CmdLeave:
			delete(t.subs, cmd.PlayerID)
		}
	}
}

// handleBet applies a betting action through the pure engine and broadcasts the
// resulting event. Server is authoritative: illegal actions are rejected here,
// never on the client.
func (t *Table) handleBet(cmd Command) {
	// TODO: decode PlaceBet, map to engine.Action, call t.Hand.Apply,
	// on success broadcast EvBetPlaced/EvStreet, on error reply EvError.
	_ = cmd
}

// broadcast sends an event to every subscriber with the next Seq.
func (t *Table) broadcast(ev protocol.Envelope) {
	t.seq++
	ev.Seq = t.seq
	for _, out := range t.subs {
		select {
		case out <- ev:
		default: // slow client: drop; it will Seq-gap and request a resync.
		}
	}
}
