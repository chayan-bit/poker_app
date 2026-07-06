package economy

import (
	"errors"
	"sync"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// ErrInsufficientFunds is returned by BuyIn when the player's balance is
// below the requested amount.
var ErrInsufficientFunds = errors.New("economy: insufficient funds")

const (
	minRefillGap    = 24 * time.Hour
	maxRefillGap    = 48 * time.Hour
	streakBonusUnit = engine.Chips(500)
)

// Ledger tracks player balances on top of a Store. It serializes
// read-modify-write sequences with an internal mutex so that concurrent
// Debit/Credit/ClaimDailyRefill calls never race in-process. A PostgreSQL
// Store implementation would instead rely on row-level locks on the
// player's balance row, and this in-process mutex would become redundant
// but harmless.
type Ledger struct {
	mu    sync.Mutex
	store Store
	now   func() time.Time
}

// NewLedger builds a Ledger backed by store, using now to source the current
// time (inject a fake clock in tests).
func NewLedger(store Store, now func() time.Time) *Ledger {
	return &Ledger{store: store, now: now}
}

// get returns the player's record, creating one with StartingBalance if
// absent. Caller must hold l.mu.
func (l *Ledger) get(playerID string) PlayerEconomy {
	pe, ok := l.store.Get(playerID)
	if !ok {
		pe = PlayerEconomy{PlayerID: playerID, Balance: StartingBalance}
	}
	return pe
}

// Balance returns the player's chips, auto-creating them with
// StartingBalance on first access.
func (l *Ledger) Balance(playerID string) engine.Chips {
	l.mu.Lock()
	defer l.mu.Unlock()
	pe := l.get(playerID)
	l.store.Put(pe)
	return pe.Balance
}

// Debit removes chips for a buy-in; returns false if insufficient.
func (l *Ledger) Debit(playerID string, amt engine.Chips) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	pe := l.get(playerID)
	if pe.Balance < amt {
		l.store.Put(pe)
		return false
	}
	pe.Balance -= amt
	l.store.Put(pe)
	return true
}

// Credit adds chips (cash-out, winnings settle-up).
func (l *Ledger) Credit(playerID string, amt engine.Chips) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pe := l.get(playerID)
	pe.Balance += amt
	l.store.Put(pe)
}

// BuyIn debits amt from playerID's balance for sitting down at a table.
// Returns ErrInsufficientFunds if the balance is too low.
func (l *Ledger) BuyIn(playerID string, amt engine.Chips) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	pe := l.get(playerID)
	if pe.Balance < amt {
		l.store.Put(pe)
		return ErrInsufficientFunds
	}
	pe.Balance -= amt
	l.store.Put(pe)
	return nil
}

// CashOut credits amt back to playerID's balance when leaving a table.
func (l *Ledger) CashOut(playerID string, amt engine.Chips) {
	l.Credit(playerID, amt)
}

// ClaimDailyRefill grants the daily allowance if at least 24h have elapsed
// since the last claim. Claims made 24-48h after the previous one extend the
// streak (adding a bonus, capped at 2x DailyRefill); a gap over 48h resets
// the streak to zero. Returns ok=false (no grant, no state change) if it is
// too early to claim.
func (l *Ledger) ClaimDailyRefill(playerID string) (granted engine.Chips, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pe := l.get(playerID)

	now := l.now()
	gap := now.Sub(pe.LastRefill)
	if gap < minRefillGap {
		l.store.Put(pe)
		return 0, false
	}

	if gap <= maxRefillGap {
		pe.Streak++
	} else {
		pe.Streak = 0
	}

	grant := DailyRefill + engine.Chips(pe.Streak)*streakBonusUnit
	if cap := 2 * DailyRefill; grant > cap {
		grant = cap
	}

	pe.Balance += grant
	pe.LastRefill = now
	l.store.Put(pe)
	return grant, true
}

// TopUpIfBroke enforces the bankruptcy floor so a player can always sit down.
func (l *Ledger) TopUpIfBroke(playerID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pe := l.get(playerID)
	if pe.Balance < BankruptcyFloor {
		pe.Balance = BankruptcyFloor
	}
	l.store.Put(pe)
}
