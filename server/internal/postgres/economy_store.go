package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// upsertEconomySQL upserts a full economy row, used by both Put and
// UpdateAtomic.
const upsertEconomySQL = `
	INSERT INTO economy (player_id, balance, last_refill, streak)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (player_id) DO UPDATE SET
		balance = EXCLUDED.balance,
		last_refill = EXCLUDED.last_refill,
		streak = EXCLUDED.streak
`

// EconomyStore is a PostgreSQL-backed economy.Store.
//
// economy.Ledger serializes its Get/Put read-modify-write sequences with an
// in-process mutex, which is only correct for a single pokerd node. Get and
// Put below are plain, unlocked reads/writes (matching the Store interface,
// which has no transactional hooks); running multiple pokerd nodes against
// the same database with the existing Ledger would race across nodes. For
// that scenario, use UpdateAtomic, which takes row-level locks in the
// database itself via SELECT ... FOR UPDATE, instead of routing through
// Ledger.
type EconomyStore struct {
	db *DB
}

var _ economy.Store = (*EconomyStore)(nil)

// NewEconomyStore builds an EconomyStore backed by db.
func NewEconomyStore(db *DB) *EconomyStore {
	return &EconomyStore{db: db}
}

// Get returns the record for playerID and whether it exists.
func (s *EconomyStore) Get(playerID string) (economy.PlayerEconomy, bool) {
	ctx, cancel := opContext()
	defer cancel()

	var pe economy.PlayerEconomy
	var balance int64
	pe.PlayerID = playerID
	err := s.db.Pool.QueryRow(ctx, `
		SELECT balance, last_refill, streak FROM economy WHERE player_id = $1
	`, playerID).Scan(&balance, &pe.LastRefill, &pe.Streak)
	if err != nil {
		return economy.PlayerEconomy{}, false
	}
	pe.Balance = engine.Chips(balance)
	return pe, true
}

// Put stores (creates or replaces) a player's record.
func (s *EconomyStore) Put(pe economy.PlayerEconomy) {
	ctx, cancel := opContext()
	defer cancel()

	// Best-effort: Put has no error return in the economy.Store interface.
	_, _ = s.db.Pool.Exec(ctx, upsertEconomySQL, pe.PlayerID, int64(pe.Balance), pe.LastRefill, pe.Streak)
}

// UpdateAtomic locks playerID's economy row with SELECT ... FOR UPDATE
// inside a transaction, applies fn to the current (or zero-value, if
// absent) record, and persists the result. Unlike Get/Put, this is safe
// against concurrent updates from multiple pokerd nodes. Reserved for
// future use once a caller needs cross-node-safe read-modify-write; the
// in-process economy.Ledger does not use this yet.
func (s *EconomyStore) UpdateAtomic(ctx context.Context, playerID string, fn func(economy.PlayerEconomy) economy.PlayerEconomy) error {
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var pe economy.PlayerEconomy
	var balance int64
	pe.PlayerID = playerID
	err = tx.QueryRow(ctx, `
		SELECT balance, last_refill, streak FROM economy WHERE player_id = $1 FOR UPDATE
	`, playerID).Scan(&balance, &pe.LastRefill, &pe.Streak)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		pe = economy.PlayerEconomy{PlayerID: playerID, Balance: economy.StartingBalance}
	case err != nil:
		return err
	default:
		pe.Balance = engine.Chips(balance)
	}

	updated := fn(pe)
	updated.PlayerID = playerID

	if _, err := tx.Exec(ctx, upsertEconomySQL,
		updated.PlayerID, int64(updated.Balance), updated.LastRefill, updated.Streak); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
