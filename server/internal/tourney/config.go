// Package tourney implements single-table tournaments (sit-and-go): a wrapper
// around exactly one table.Table (2-9 players) that owns everything the table
// deliberately does not - the blind clock, elimination ordering, and prize-pool
// payouts. The table exposes one additive callback (table.Deps.OnHandComplete);
// this package is the brain behind it. See internal/table/tourney.go for the
// seam.
package tourney

import (
	"errors"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// Level is one rung of the blind schedule: the small/big blind in force and how
// long the level lasts before the clock advances.
type Level struct {
	SmallBlind engine.Chips
	BigBlind   engine.Chips
	Duration   time.Duration
}

// SNGConfig is the full ruleset for a sit-and-go.
type SNGConfig struct {
	Name          string
	Seats         int          // 2..9
	BuyIn         engine.Chips // cash chips debited from the ledger at registration
	StartingStack engine.Chips // tournament chips each seat starts with
	BlindLevels   []Level      // clock-driven schedule (non-empty)
	PayoutPct     []int        // prize split, summing to 100
}

// Defaults.
const (
	DefaultStartingStack engine.Chips  = 1500
	DefaultLevelDuration time.Duration = 5 * time.Minute
)

var (
	ErrBadSeats      = errors.New("tourney: seats must be between 2 and 9")
	ErrBadBuyIn      = errors.New("tourney: buy-in must be positive")
	ErrBadStack      = errors.New("tourney: starting stack must be positive")
	ErrNoLevels      = errors.New("tourney: blind schedule must be non-empty")
	ErrBadPayout     = errors.New("tourney: payout percentages must sum to 100")
	ErrTooManyPayout = errors.New("tourney: more payout places than seats")
)

// DefaultBlindSchedule is a sane escalating schedule at DefaultLevelDuration per
// level. Blinds roughly double every couple of levels, the standard SNG ramp.
func DefaultBlindSchedule() []Level {
	steps := [][2]engine.Chips{
		{10, 20}, {15, 30}, {25, 50}, {50, 100}, {75, 150}, {100, 200},
		{150, 300}, {200, 400}, {300, 600}, {400, 800}, {600, 1200}, {1000, 2000},
	}
	out := make([]Level, len(steps))
	for i, s := range steps {
		out[i] = Level{SmallBlind: s[0], BigBlind: s[1], Duration: DefaultLevelDuration}
	}
	return out
}

// DefaultPayoutPct returns the standard prize split for a field of n players:
//   - 2-4 players: winner takes all         -> [100]
//   - 5-6 players: top two                   -> [65, 35]
//   - 7-9 players: top three                 -> [50, 30, 20]
func DefaultPayoutPct(n int) []int {
	switch {
	case n <= 4:
		return []int{100}
	case n <= 6:
		return []int{65, 35}
	default:
		return []int{50, 30, 20}
	}
}

// DefaultConfig builds an SNGConfig for a plain create request (name, seats,
// buy-in), filling the starting stack, blind schedule, and payout table with
// sensible defaults.
func DefaultConfig(name string, seats int, buyIn engine.Chips) SNGConfig {
	return SNGConfig{
		Name:          name,
		Seats:         seats,
		BuyIn:         buyIn,
		StartingStack: DefaultStartingStack,
		BlindLevels:   DefaultBlindSchedule(),
		PayoutPct:     DefaultPayoutPct(seats),
	}
}

// validate checks an SNGConfig is internally consistent.
func (c SNGConfig) validate() error {
	if c.Seats < 2 || c.Seats > 9 {
		return ErrBadSeats
	}
	if c.BuyIn <= 0 {
		return ErrBadBuyIn
	}
	if c.StartingStack <= 0 {
		return ErrBadStack
	}
	if len(c.BlindLevels) == 0 {
		return ErrNoLevels
	}
	if len(c.PayoutPct) > c.Seats {
		return ErrTooManyPayout
	}
	sum := 0
	for _, p := range c.PayoutPct {
		sum += p
	}
	if sum != 100 {
		return ErrBadPayout
	}
	return nil
}
