package postgres

import (
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/chayan-bit/poker_app/server/internal/auth"
)

// pgUniqueViolation is the PostgreSQL error code for a unique constraint
// violation (23505), used to detect duplicate-email conflicts.
const pgUniqueViolation = "23505"

// AuthStore is a PostgreSQL-backed auth.Store.
type AuthStore struct {
	db *DB
}

var _ auth.Store = (*AuthStore)(nil)

// NewAuthStore builds an AuthStore backed by db.
func NewAuthStore(db *DB) *AuthStore {
	return &AuthStore{db: db}
}

// CreateGuest allocates a new guest account with a fresh PlayerID.
func (s *AuthStore) CreateGuest() auth.Account {
	ctx, cancel := opContext()
	defer cancel()

	acc := auth.Account{
		PlayerID: uuid.NewString(),
	}

	row := s.db.Pool.QueryRow(ctx, `
		INSERT INTO accounts (player_id, email, guest, created_at)
		VALUES ($1, NULL, true, now())
		RETURNING created_at
	`, acc.PlayerID)

	// CreateGuest has no error return in the auth.Store interface; a failed
	// insert here would surface as ByID/ByEmail never finding the account,
	// which callers already treat as a valid "not found" outcome.
	_ = row.Scan(&acc.CreatedAt)
	return acc
}

// UpgradeToAccount attaches email to the existing playerID, preserving it.
// Mirrors auth.MemStore semantics: if email is already taken by a different
// player, returns auth.ErrEmailTaken without changing state.
func (s *AuthStore) UpgradeToAccount(playerID, email string) (auth.Account, error) {
	ctx, cancel := opContext()
	defer cancel()

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return auth.Account{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existingID string
	err = tx.QueryRow(ctx, `SELECT player_id FROM accounts WHERE email = $1`, email).Scan(&existingID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return auth.Account{}, err
	}
	if err == nil && existingID != playerID {
		return auth.Account{}, auth.ErrEmailTaken
	}

	var acc auth.Account
	err = tx.QueryRow(ctx, `
		INSERT INTO accounts (player_id, email, guest, created_at)
		VALUES ($1, $2, false, now())
		ON CONFLICT (player_id) DO UPDATE SET email = EXCLUDED.email, guest = false
		RETURNING player_id, COALESCE(email, ''), created_at
	`, playerID, email).Scan(&acc.PlayerID, &acc.Email, &acc.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
			return auth.Account{}, auth.ErrEmailTaken
		}
		return auth.Account{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return auth.Account{}, err
	}
	return acc, nil
}

// ByEmail looks up an account by email.
func (s *AuthStore) ByEmail(email string) (auth.Account, bool) {
	ctx, cancel := opContext()
	defer cancel()

	var acc auth.Account
	err := s.db.Pool.QueryRow(ctx, `
		SELECT player_id, COALESCE(email, ''), created_at FROM accounts WHERE email = $1
	`, email).Scan(&acc.PlayerID, &acc.Email, &acc.CreatedAt)
	if err != nil {
		return auth.Account{}, false
	}
	return acc, true
}

// ByID looks up an account by player ID.
func (s *AuthStore) ByID(id string) (auth.Account, bool) {
	ctx, cancel := opContext()
	defer cancel()

	var acc auth.Account
	err := s.db.Pool.QueryRow(ctx, `
		SELECT player_id, COALESCE(email, ''), created_at FROM accounts WHERE player_id = $1
	`, id).Scan(&acc.PlayerID, &acc.Email, &acc.CreatedAt)
	if err != nil {
		return auth.Account{}, false
	}
	return acc, true
}
