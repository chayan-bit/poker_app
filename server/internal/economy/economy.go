// Package economy implements the fake-money system: daily refills, a bankruptcy
// floor so no one is ever locked out, and streak bonuses (Design_suite 6.4).
// There is NO purchase path by design; chips are a scoreboard.
//
// All balances are integer engine.Chips. This scaffold uses an in-memory store;
// swap for PostgreSQL with row-level locking on the balance to prevent double
// spends across concurrent tables.
package economy

import "github.com/chayan-bit/poker_app/server/internal/engine"

// Balances of the fake economy.
const (
	StartingBalance engine.Chips = 10_000
	DailyRefill     engine.Chips = 5_000
	BankruptcyFloor engine.Chips = 1_000 // topped up to this if broke
)

// Ledger tracks player balances. Replace with a persistent, transactional store.
type Ledger struct {
	balances map[string]engine.Chips
}

// NewLedger builds an empty in-memory ledger.
func NewLedger() *Ledger { return &Ledger{balances: map[string]engine.Chips{}} }

// Balance returns the player's chips, creating them with StartingBalance.
func (l *Ledger) Balance(playerID string) engine.Chips {
	if _, ok := l.balances[playerID]; !ok {
		l.balances[playerID] = StartingBalance
	}
	return l.balances[playerID]
}

// Debit removes chips for a buy-in; returns false if insufficient.
func (l *Ledger) Debit(playerID string, amt engine.Chips) bool {
	if l.Balance(playerID) < amt {
		return false
	}
	l.balances[playerID] -= amt
	return true
}

// Credit adds chips (cash-out, winnings settle-up).
func (l *Ledger) Credit(playerID string, amt engine.Chips) {
	l.balances[playerID] = l.Balance(playerID) + amt
}

// ClaimDailyRefill grants the daily allowance. Caller enforces once-per-day via
// a persisted last-claim timestamp (no clock in this pure scaffold).
func (l *Ledger) ClaimDailyRefill(playerID string) engine.Chips {
	l.Credit(playerID, DailyRefill)
	return l.Balance(playerID)
}

// TopUpIfBroke enforces the bankruptcy floor so a player can always sit down.
func (l *Ledger) TopUpIfBroke(playerID string) {
	if l.Balance(playerID) < BankruptcyFloor {
		l.balances[playerID] = BankruptcyFloor
	}
}
