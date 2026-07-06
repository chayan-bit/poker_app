// Package economy implements the fake-money system: daily refills, a bankruptcy
// floor so no one is ever locked out, and streak bonuses (Design_suite 6.4).
// There is NO purchase path by design; chips are a scoreboard.
//
// All balances are integer engine.Chips. Balances are held behind the Store
// interface (store.go); this scaffold ships an in-memory implementation.
// Swap in a PostgreSQL-backed Store with row-level locking on the balance
// row to prevent double spends across concurrent tables - the Ledger itself
// only serializes access within a single process via an internal mutex.
package economy

import "github.com/chayan-bit/poker_app/server/internal/engine"

// Balances of the fake economy.
const (
	StartingBalance engine.Chips = 10_000
	DailyRefill     engine.Chips = 5_000
	BankruptcyFloor engine.Chips = 1_000 // topped up to this if broke
)
