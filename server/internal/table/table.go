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

// Timeout defaults used when the matching Deps field is zero.
const (
	defaultTurnTimeout     = 20 * time.Second
	defaultIdleTimeout     = 5 * time.Minute
	defaultDisconnectGrace = 30 * time.Second
	brokeUnseatAfterHands  = 3 // hands started while broke before auto-unseat
)

// Config is the host-chosen ruleset for a table (see Design_suite 6.2).
type Config struct {
	ID         string
	Visibility Visibility
	MaxSeats   int
	SmallBlind engine.Chips
	BigBlind   engine.Chips
	JoinCode   string // 6-char code for private rooms
	// AutoStart deals hands automatically once two seats are ready. Public tables
	// are always auto-start; private rooms default to host-started (AutoStart
	// false), where the host must send start_hand once before hands auto-continue.
	AutoStart bool
	// HostPlayerID is the private room's host. If empty, the first player to sit
	// becomes host (the lobby may set it explicitly later).
	HostPlayerID string
	// Tournament, when non-nil, runs this table as a single-table tournament
	// (sit-and-go). See tourney.go for the additive seam; nil means a normal
	// cash table with fully unchanged behaviour.
	Tournament *TourneyRules
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
	// IdleTimeout is how long a table with 0 subscribers AND 0 seated players
	// stays alive before it shuts down and removes itself from the registry.
	IdleTimeout time.Duration
	// DisconnectGrace is how long a seated player's socket may stay dropped
	// before the seat is sat out (their turn timer still runs meanwhile).
	DisconnectGrace time.Duration
	// OnShutdown, if set, is invoked (from the table goroutine) with the table ID
	// when the table shuts down on idle. The registry wires this to Remove.
	OnShutdown func(tableID string)
	// OnHandComplete, if set, is invoked (from the table goroutine) after every
	// settled hand in tournament mode. It returns a directive that raises blinds,
	// removes busted seats, and ends the tournament. Ignored on cash tables
	// (Config.Tournament == nil). See tourney.go.
	OnHandComplete OnHandComplete
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
	if d.IdleTimeout <= 0 {
		d.IdleTimeout = defaultIdleTimeout
	}
	if d.DisconnectGrace <= 0 {
		d.DisconnectGrace = defaultDisconnectGrace
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
	// foldPending marks a seat that requested sit_out mid-hand while it was not
	// their turn: they auto-fold when action reaches them (cleared each deal).
	foldPending bool
	// disconnected + graceDeadline drive the disconnect-grace flow. While
	// disconnected the seat still runs the normal turn timer; if grace expires
	// still disconnected the seat is sat out (not unseated).
	disconnected  bool
	graceDeadline time.Time
	// brokeAtHand is the hand number at which this seat first hit 0 chips (0 when
	// not broke). Used to auto-unseat after brokeUnseatAfterHands more hands.
	brokeAtHand int
}

// Table is the live state. Only its own goroutine touches these fields.
type Table struct {
	Cfg  Config
	Hand *engine.HandState

	deps  Deps
	seats map[int]*seatState // seatID -> seat state
	subs  map[string]chan<- protocol.Envelope
	inbox chan Command
	seq   uint64

	timer   Timer
	button  int // seat ID of the dealer button
	handNum int

	// hostStarted flips true after the first start_hand in a non-auto-start room;
	// thereafter hands auto-continue.
	hostStarted bool
	// autoFolding guards driveAutoFolds against re-entrancy.
	autoFolding bool
	// tourneyDone latches true when a tournament completes, so no further hands
	// deal. Always false on cash tables.
	tourneyDone bool

	// deadlines the loop multiplexes onto its single timer (zero == disarmed).
	turnDeadline time.Time
	idleDeadline time.Time
	// done is closed when loop() exits (idle shutdown), for deterministic tests.
	done    chan struct{}
	stopped bool

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
		done:       make(chan struct{}),
	}
	// Tournament tables pre-seat their registered players at the starting stack
	// before the loop runs; the first hand deals once those players connect.
	if cfg.Tournament != nil {
		t.seatTournamentPlayers()
	}
	// A brand-new table has no subscribers or seats, so start the idle clock.
	// Safe to arm before the goroutine starts: nothing else touches the timer yet.
	t.refreshIdle()
	go t.loop()
	return t
}

// Done is closed once the table's loop has exited (idle shutdown). Tests use it
// to assert the goroutine terminated.
func (t *Table) Done() <-chan struct{} { return t.done }

// autoStart reports whether hands deal automatically: always for public tables,
// and for private rooms only once the host has started the first hand (or opted
// in via Config.AutoStart).
func (t *Table) autoStart() bool {
	return t.Cfg.AutoStart || t.Cfg.Visibility == Public
}

// NewWithDefaults builds a table with all-default dependencies. Kept as a
// backward-compatible constructor for callers that do not wire deps.
func NewWithDefaults(cfg Config) *Table { return New(cfg, Deps{}) }

// Submit enqueues a command; non-blocking beyond the buffered inbox.
func (t *Table) Submit(c Command) { t.inbox <- c }

// loop is the single-threaded owner of table state. It multiplexes inbound
// commands with turn-timer expiries so both mutate state on the one goroutine.
func (t *Table) loop() {
	defer close(t.done)
	for {
		select {
		case cmd, ok := <-t.inbox:
			if !ok {
				return
			}
			t.handle(cmd)
		case <-t.timer.C():
			t.onTimer()
		}
		if t.stopped {
			return
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
		t.sendTo(cmd.Reply, protocol.EvSnapshot, t.snapshotFor(cmd.PlayerID))
	case protocol.CmdLeave:
		t.handleLeave(cmd)
	case protocol.CmdStartHand:
		t.handleStartHand(cmd)
	case protocol.CmdRebuy:
		t.handleRebuy(cmd)
	case protocol.CmdSitOut:
		t.handleSitOut(cmd)
	case protocol.CmdSitIn:
		t.handleSitIn(cmd)
	case protocol.CmdDisconnected:
		t.handleDisconnect(cmd)
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
