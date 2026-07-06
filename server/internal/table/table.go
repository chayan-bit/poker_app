// Package table owns live table state and the per-table event loop. Each table
// runs in its own goroutine and is the single writer of its state, so the pure
// engine never needs locks. Clients talk to a table only via its command chan.
//
// Concurrency model: every field on Table is touched only by loop(). Inbound
// work arrives on the buffered inbox; outbound events leave through per-
// connection channels held in subs. Because there is exactly one writer, no
// mutex guards game state.
package table

import (
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/history"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// Visibility controls who may join.
type Visibility uint8

const (
	Public Visibility = iota
	Private
)

// defaultTurnTimeout is the per-action deadline when Deps.TurnTimeout is zero.
const defaultTurnTimeout = 20 * time.Second

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

// Deps are the external collaborators a table needs to run real hands. They are
// injected so tests can supply fakes (deterministic clock, in-memory ledger).
type Deps struct {
	Ledger      *economy.Ledger // buy-in / cash-out of real chip balances
	History     history.Store   // durable hand records
	Now         func() time.Time
	TurnTimeout time.Duration
	Clock       Clock // timer factory; injected so turn deadlines are testable
}

// withDefaults fills any zero field with a production default so a table always
// has a working ledger, history store, clock, and timeout.
func (d Deps) withDefaults() Deps {
	if d.Clock == nil {
		d.Clock = realClock{}
	}
	if d.Now == nil {
		d.Now = d.Clock.Now
	}
	if d.TurnTimeout <= 0 {
		d.TurnTimeout = defaultTurnTimeout
	}
	if d.Ledger == nil {
		d.Ledger = economy.NewLedger(economy.NewMemoryStore(), d.Now)
	}
	if d.History == nil {
		d.History = history.NewMemStore()
	}
	return d
}

// Command is an inbound client request routed to the table goroutine.
type Command struct {
	PlayerID string
	Msg      protocol.Envelope
	Reply    chan<- protocol.Envelope // per-connection outbound
}

// seatState is one seat's durable, cross-hand state (owned by loop()).
type seatState struct {
	playerID   string
	stack      engine.Chips // chips at the seat between hands
	sittingOut bool         // excluded from the next deal (e.g. timed out)
}

// Table is the live state. Only its own goroutine touches these fields.
type Table struct {
	Cfg  Config
	Hand *engine.HandState

	deps  Deps
	seats map[int]*seatState               // seatID -> seat state
	subs  map[string]chan<- protocol.Envelope
	inbox chan Command
	seq   uint64

	timer   Timer
	button  int // seat ID of the dealer button
	handNum int

	// per-running-hand fairness + recording state
	commitment string
	seedHex    string
	handID     string
	rec        *history.Recorder
	startStack map[int]engine.Chips // seat -> stack at hand start (for records)
}

// New builds a table with explicit dependencies and starts its event loop.
// Missing deps are filled with production defaults, so New is always safe to
// call with a partial (or zero) Deps.
func New(cfg Config, deps Deps) *Table {
	d := deps.withDefaults()
	t := &Table{
		Cfg:        cfg,
		deps:       d,
		seats:      map[int]*seatState{},
		subs:       map[string]chan<- protocol.Envelope{},
		inbox:      make(chan Command, 64),
		timer:      d.Clock.NewTimer(),
		startStack: map[int]engine.Chips{},
	}
	go t.loop()
	return t
}

// NewWithDefaults builds a table with all-default dependencies. Kept as a
// backward-compatible constructor for callers that do not wire deps.
func NewWithDefaults(cfg Config) *Table { return New(cfg, Deps{}) }

// Submit enqueues a command; non-blocking beyond the buffered inbox.
func (t *Table) Submit(c Command) { t.inbox <- c }

// loop is the single-threaded owner of table state. It multiplexes inbound
// commands with turn-timer expiries so both mutate state on the one goroutine.
func (t *Table) loop() {
	for {
		select {
		case cmd, ok := <-t.inbox:
			if !ok {
				return
			}
			t.handle(cmd)
		case <-t.timer.C():
			t.onTurnTimeout()
		}
	}
}

// handle dispatches one inbound command.
func (t *Table) handle(cmd Command) {
	switch cmd.Msg.Type {
	case protocol.CmdJoinTable:
		t.handleJoin(cmd)
	case protocol.CmdSitDown:
		t.handleSitDown(cmd)
	case protocol.CmdPlaceBet:
		t.handleBet(cmd)
	case protocol.CmdResync:
		t.sendTo(cmd.Reply, protocol.EvSnapshot, t.snapshot())
	case protocol.CmdLeave:
		t.handleLeave(cmd)
	}
}

// nextSeq returns the next monotonic sequence number for an outbound event.
func (t *Table) nextSeq() uint64 { t.seq++; return t.seq }

// envelope wraps a payload into a versioned, sequenced Envelope.
func (t *Table) envelope(typ string, data any) protocol.Envelope {
	return protocol.Envelope{
		V:    protocol.ProtocolVersion,
		Type: typ,
		Seq:  t.nextSeq(),
		Data: mustJSON(data),
	}
}

// trySend delivers ev without blocking; a slow client is dropped and will
// Seq-gap and request a resync (see gateway outbound buffer).
func trySend(out chan<- protocol.Envelope, ev protocol.Envelope) {
	select {
	case out <- ev:
	default:
	}
}

// sendTo delivers one event to a single connection with its own Seq (used for
// per-player privacy sends: hole cards, targeted snapshots, errors).
func (t *Table) sendTo(out chan<- protocol.Envelope, typ string, data any) {
	trySend(out, t.envelope(typ, data))
}

// broadcast sends one event to every subscriber sharing a single Seq, so a
// client can detect gaps in the table-wide stream and resync.
func (t *Table) broadcast(typ string, data any) {
	ev := t.envelope(typ, data)
	for _, out := range t.subs {
		trySend(out, ev)
	}
}

// sendError replies with a non-fatal error event to a single connection.
func (t *Table) sendError(out chan<- protocol.Envelope, code, msg string) {
	t.sendTo(out, protocol.EvError, protocol.ErrorEvent{Code: code, Message: msg})
}

// seatOf returns the seat ID occupied by playerID, if any.
func (t *Table) seatOf(playerID string) (int, bool) {
	for id, s := range t.seats {
		if s.playerID == playerID {
			return id, true
		}
	}
	return 0, false
}
