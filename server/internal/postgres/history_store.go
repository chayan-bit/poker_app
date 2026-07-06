package postgres

import (
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/chayan-bit/poker_app/server/internal/history"
)

// HistoryStore is a PostgreSQL-backed history.Store.
type HistoryStore struct {
	db *DB
}

var _ history.Store = (*HistoryStore)(nil)

// NewHistoryStore builds a HistoryStore backed by db.
func NewHistoryStore(db *DB) *HistoryStore {
	return &HistoryStore{db: db}
}

// Save persists a HandRecord, keyed by its HandID, and refreshes the
// hand_players rows used by ByPlayer.
func (s *HistoryStore) Save(rec history.HandRecord) error {
	ctx, cancel := opContext()
	defer cancel()

	payload, err := json.Marshal(rec)
	if err != nil {
		return err
	}

	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO hands (hand_id, table_id, started_at, record)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (hand_id) DO UPDATE SET
			table_id = EXCLUDED.table_id,
			started_at = EXCLUDED.started_at,
			record = EXCLUDED.record
	`, rec.HandID, rec.TableID, rec.StartedAt, payload); err != nil {
		return err
	}

	// Replace hand_players wholesale: simplest way to keep it consistent
	// with rec.Seats on repeated Save calls for the same hand.
	if _, err := tx.Exec(ctx, `DELETE FROM hand_players WHERE hand_id = $1`, rec.HandID); err != nil {
		return err
	}
	for _, seat := range rec.Seats {
		if _, err := tx.Exec(ctx, `
			INSERT INTO hand_players (hand_id, player_id, started_at)
			VALUES ($1, $2, $3)
		`, rec.HandID, seat.PlayerID, rec.StartedAt); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// Get retrieves a HandRecord by HandID.
func (s *HistoryStore) Get(handID string) (history.HandRecord, bool) {
	ctx, cancel := opContext()
	defer cancel()

	var payload []byte
	err := s.db.Pool.QueryRow(ctx, `SELECT record FROM hands WHERE hand_id = $1`, handID).Scan(&payload)
	if err != nil {
		return history.HandRecord{}, false
	}

	var rec history.HandRecord
	if err := json.Unmarshal(payload, &rec); err != nil {
		return history.HandRecord{}, false
	}
	return rec, true
}

// ByPlayer returns up to limit HandRecords that include playerID as a seat,
// most-recent-first (by StartedAt). limit <= 0 means no limit.
func (s *HistoryStore) ByPlayer(playerID string, limit int) []history.HandRecord {
	ctx, cancel := opContext()
	defer cancel()

	query := `
		SELECT h.record
		FROM hand_players hp
		JOIN hands h ON h.hand_id = hp.hand_id
		WHERE hp.player_id = $1
		ORDER BY hp.started_at DESC
	`
	args := []any{playerID}
	if limit > 0 {
		query += ` LIMIT $2`
		args = append(args, limit)
	}

	rows, err := s.db.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var recs []history.HandRecord
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return recs
		}
		var rec history.HandRecord
		if err := json.Unmarshal(payload, &rec); err != nil {
			continue
		}
		recs = append(recs, rec)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return recs
	}
	return recs
}
