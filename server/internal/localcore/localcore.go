// Package localcore is a pure, synchronous, deterministic facade over the poker
// core (internal/engine + internal/fair + the pure rule parts of internal/table)
// intended to be compiled to WebAssembly and run on a player's own device.
//
// Unlike internal/table, it runs NO goroutines, opens NO channels, reads NO
// wall clock, and imports nothing from net/http, database/sql, or pgx. All time
// enters through explicit nowMs inputs (Tick and time-carrying commands); all
// randomness enters through an externally supplied seed. This makes an offline
// hand verifiably identical to an online one and safe to replicate across peers.
//
// The wire protocol is exactly internal/protocol: it accepts the same client to
// server command envelopes the WebSocket path uses and returns the resulting
// per-recipient event envelopes. The event Data shapes below mirror the ones the
// table package emits so a client renders offline and online hands identically.
package localcore

import (
	"sort"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// Broadcast is the recipient key under which table-wide events are collected in
// the output of Submit/Tick/VoidHand. Per-player events are keyed by player ID.
const Broadcast = "*"

// Default timing used when the matching Config field is zero. Durations are in
// milliseconds because all time crosses the JS boundary as integers.
const (
	defaultTurnTimeoutMs     int64 = 20_000
	defaultDisconnectGraceMs int64 = 30_000
)

// Config is the host-chosen ruleset for a local table. It mirrors the pure
// subset of table.Config and is JSON-friendly for the WASM boundary. Tournament,
// ledger, and history settings are intentionally absent: offline play is a
// cash table with per-session fun chips (see issue #27 scope).
type Config struct {
	ID           string `json:"id"`
	MaxSeats     int    `json:"maxSeats"`
	SmallBlind   int64  `json:"smallBlind"`
	BigBlind     int64  `json:"bigBlind"`
	HostPlayerID string `json:"hostPlayerId"`
	// AutoStart deals hands automatically once two seats are ready. When false
	// (a private room), the host must send start_hand once before hands
	// auto-continue.
	AutoStart bool `json:"autoStart"`
	// Private marks a host-started room. Public tables always auto-start.
	Private bool `json:"private"`
	// TurnTimeoutMs / DisconnectGraceMs default when zero.
	TurnTimeoutMs     int64 `json:"turnTimeoutMs"`
	DisconnectGraceMs int64 `json:"disconnectGraceMs"`
}

func (c Config) withDefaults() Config {
	if c.TurnTimeoutMs <= 0 {
		c.TurnTimeoutMs = defaultTurnTimeoutMs
	}
	if c.DisconnectGraceMs <= 0 {
		c.DisconnectGraceMs = defaultDisconnectGraceMs
	}
	if c.MaxSeats <= 0 {
		c.MaxSeats = 9
	}
	return c
}

// seatState is one seat's durable, cross-hand state. It is a pure-data mirror of
// table.seatState with the wall-clock grace deadline replaced by an int64 ms.
type seatState struct {
	playerID    string
	stack       engine.Chips
	sittingOut  bool
	foldPending bool
	// disconnected + graceDeadlineMs drive the disconnect-grace flow, expressed
	// entirely in externally supplied milliseconds.
	disconnected    bool
	graceDeadlineMs int64
}

// LocalTable is the synchronous authoritative state of one table. Every field is
// touched only by the calling goroutine (there is exactly one, by construction);
// no channels, no timers, no locks.
type LocalTable struct {
	cfg  Config
	hand *engine.HandState

	seats     map[int]*seatState
	connected map[string]bool // players that have joined and can receive hole cards
	seq       uint64

	button  int
	handNum int

	hostStarted bool
	autoFolding bool

	// Time is external. nowMs is the most recent timestamp observed from any
	// input; turnDeadlineMs (0 == disarmed) is when the current actor's turn
	// expires. Both are plain integers, never derived from a local clock.
	nowMs          int64
	turnDeadlineMs int64

	// pendingSeed is the externally supplied 32-byte hex seed to use for the NEXT
	// hand. It is consumed on deal (a hand never deals twice off one seed) so the
	// coordinator must supply a fresh seed per hand via setSeed.
	pendingSeed string

	// per-running-hand fairness state
	commitment string
	seedHex    string
	handID     string
	// startStack records each dealt seat's stack at hand start so VoidHand can
	// return every committed chip of an aborted hand.
	startStack map[int]engine.Chips
}

// NewLocalTable builds a table ready to receive commands. seedHex is the seed
// for the first hand (may be empty and supplied later via SetSeed); it is used
// exactly as internal/fair does online, so offline hands verify identically.
func NewLocalTable(cfg Config, seedHex string) *LocalTable {
	return &LocalTable{
		cfg:         cfg.withDefaults(),
		seats:       map[int]*seatState{},
		connected:   map[string]bool{},
		startStack:  map[int]engine.Chips{},
		pendingSeed: seedHex,
	}
}

// SetSeed sets the seed to be committed and shuffled for the next hand.
func (lt *LocalTable) SetSeed(seedHex string) { lt.pendingSeed = seedHex }

// autoStart reports whether hands deal automatically: always for public tables,
// and for private rooms only once the host has started the first hand.
func (lt *LocalTable) autoStart() bool {
	return lt.cfg.AutoStart || !lt.cfg.Private
}

// nextSeq returns the next monotonic sequence number for an outbound event.
func (lt *LocalTable) nextSeq() uint64 { lt.seq++; return lt.seq }

// seatOf returns the seat ID occupied by playerID, if any.
func (lt *LocalTable) seatOf(playerID string) (int, bool) {
	for id, s := range lt.seats {
		if s.playerID == playerID {
			return id, true
		}
	}
	return 0, false
}

// handIndex returns the index of seat within the live hand, or -1.
func (lt *LocalTable) handIndex(seat int) int {
	if lt.hand == nil {
		return -1
	}
	for i, p := range lt.hand.Players {
		if p.SeatID == seat {
			return i
		}
	}
	return -1
}

// sortedSeatIDs returns occupied seat IDs in ascending order (deterministic
// iteration for hashing and payload building; Go map order is not stable).
func (lt *LocalTable) sortedSeatIDs() []int {
	ids := make([]int, 0, len(lt.seats))
	for id := range lt.seats {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}
