package postgres

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/chayan-bit/poker_app/server/internal/auth"
	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/history"
)

// TestLoadMigrations_ParseAndOrder is the one always-run test: it needs no
// database, just validates the embedded migration files parse and come out
// version-ordered with no gaps in ordering logic.
func TestLoadMigrations_ParseAndOrder(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations: %v", err)
	}
	if len(migs) == 0 {
		t.Fatal("expected at least one migration")
	}
	for i := 1; i < len(migs); i++ {
		if migs[i].version <= migs[i-1].version {
			t.Fatalf("migrations not strictly ordered: %d then %d", migs[i-1].version, migs[i].version)
		}
	}
	for _, m := range migs {
		if m.sql == "" {
			t.Fatalf("migration %s has empty SQL", m.name)
		}
	}
}

// testDB spins up an isolated schema (search_path) inside the database
// pointed to by POKERD_TEST_DB, runs Migrate against it, and returns a DB
// plus a cleanup func that drops the schema. Skips the calling test if
// POKERD_TEST_DB is unset.
func testDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("POKERD_TEST_DB")
	if dsn == "" {
		t.Skip("POKERD_TEST_DB not set; skipping real-database test")
	}

	ctx := context.Background()

	// Create the schema using a throwaway connection before configuring the
	// pool, since the pool below pins search_path per-connection via
	// AfterConnect and every pooled connection must see the schema already
	// existing.
	schema := fmt.Sprintf("pokerd_test_%s", uuid.NewString()[:8])
	setupDB, err := Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("Connect (setup): %v", err)
	}
	if _, err := setupDB.Pool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %q`, schema)); err != nil {
		setupDB.Close()
		t.Fatalf("create schema: %v", err)
	}
	setupDB.Close()

	// search_path is a per-connection session setting; pgxpool multiplexes
	// many physical connections, so a plain "SET search_path" on one
	// borrowed connection would not apply to the others. AfterConnect runs
	// on every new physical connection the pool opens, which is the
	// correct way to pin search_path pool-wide.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, fmt.Sprintf(`SET search_path TO %q`, schema))
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("NewWithConfig: %v", err)
	}
	db := &DB{Pool: pool}

	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema))
		db.Close()
	})

	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestAuthStore_GuestCreateUpgradeDuplicateEmail(t *testing.T) {
	db := testDB(t)
	store := NewAuthStore(db)

	guest := store.CreateGuest()
	if guest.PlayerID == "" {
		t.Fatal("expected non-empty player ID for guest")
	}

	upgraded, err := store.UpgradeToAccount(guest.PlayerID, "alice@example.com")
	if err != nil {
		t.Fatalf("UpgradeToAccount: %v", err)
	}
	if upgraded.PlayerID != guest.PlayerID {
		t.Fatalf("player ID changed on upgrade: %s -> %s", guest.PlayerID, upgraded.PlayerID)
	}
	if upgraded.Email != "alice@example.com" {
		t.Fatalf("email = %q, want alice@example.com", upgraded.Email)
	}

	byEmail, ok := store.ByEmail("alice@example.com")
	if !ok || byEmail.PlayerID != guest.PlayerID {
		t.Fatalf("ByEmail lookup failed: %+v, ok=%v", byEmail, ok)
	}

	byID, ok := store.ByID(guest.PlayerID)
	if !ok || byID.Email != "alice@example.com" {
		t.Fatalf("ByID lookup failed: %+v, ok=%v", byID, ok)
	}

	other := store.CreateGuest()
	if _, err := store.UpgradeToAccount(other.PlayerID, "alice@example.com"); err != auth.ErrEmailTaken {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
}

func TestEconomyStore_GetPutRoundtrip(t *testing.T) {
	db := testDB(t)
	store := NewEconomyStore(db)

	playerID := uuid.NewString()
	if _, ok := store.Get(playerID); ok {
		t.Fatal("expected no record before Put")
	}

	want := economy.PlayerEconomy{
		PlayerID:   playerID,
		Balance:    engine.Chips(12_345),
		LastRefill: time.Now().UTC().Truncate(time.Microsecond),
		Streak:     3,
	}
	store.Put(want)

	got, ok := store.Get(playerID)
	if !ok {
		t.Fatal("expected record after Put")
	}
	if got.Balance != want.Balance || got.Streak != want.Streak {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if !got.LastRefill.Equal(want.LastRefill) {
		t.Fatalf("LastRefill = %v, want %v", got.LastRefill, want.LastRefill)
	}
}

func TestEconomyStore_UpdateAtomicConcurrency(t *testing.T) {
	db := testDB(t)
	store := NewEconomyStore(db)
	ctx := context.Background()

	playerID := uuid.NewString()
	store.Put(economy.PlayerEconomy{PlayerID: playerID, Balance: 0})

	const increments = 20
	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	incrementOnce := func() {
		defer wg.Done()
		for i := 0; i < increments; i++ {
			err := store.UpdateAtomic(ctx, playerID, func(pe economy.PlayerEconomy) economy.PlayerEconomy {
				pe.Balance++
				return pe
			})
			if err != nil {
				errCh <- err
				return
			}
		}
	}

	wg.Add(2)
	go incrementOnce()
	go incrementOnce()
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("UpdateAtomic: %v", err)
	}

	got, ok := store.Get(playerID)
	if !ok {
		t.Fatal("expected record after concurrent updates")
	}
	if got.Balance != 2*increments {
		t.Fatalf("balance = %d, want %d (lost update under concurrency)", got.Balance, 2*increments)
	}
}

func TestHistoryStore_SaveGetByPlayer(t *testing.T) {
	db := testDB(t)
	store := NewHistoryStore(db)

	playerA := uuid.NewString()
	playerB := uuid.NewString()
	base := time.Now().UTC().Truncate(time.Millisecond)

	makeRecord := func(handID string, startedAt time.Time, players ...string) history.HandRecord {
		var seats []history.SeatInfo
		for i, p := range players {
			seats = append(seats, history.SeatInfo{SeatID: i, PlayerID: p, StartStack: 10_000})
		}
		return history.HandRecord{
			HandID:    handID,
			TableID:   "table-1",
			StartedAt: startedAt,
			Seats:     seats,
			Board:     []string{"As", "Kd", "2c"},
			Results:   map[int]string{0: "won 100"},
		}
	}

	rec1 := makeRecord("hand-1", base, playerA, playerB)
	rec2 := makeRecord("hand-2", base.Add(time.Minute), playerA)
	rec3 := makeRecord("hand-3", base.Add(2*time.Minute), playerA)

	for _, rec := range []history.HandRecord{rec1, rec2, rec3} {
		if err := store.Save(rec); err != nil {
			t.Fatalf("Save(%s): %v", rec.HandID, err)
		}
	}

	got, ok := store.Get("hand-1")
	if !ok {
		t.Fatal("expected hand-1 to exist")
	}
	if got.TableID != "table-1" || len(got.Seats) != 2 {
		t.Fatalf("Get(hand-1) = %+v", got)
	}

	byPlayerA := store.ByPlayer(playerA, 0)
	if len(byPlayerA) != 3 {
		t.Fatalf("ByPlayer(playerA) len = %d, want 3", len(byPlayerA))
	}
	if byPlayerA[0].HandID != "hand-3" || byPlayerA[1].HandID != "hand-2" || byPlayerA[2].HandID != "hand-1" {
		t.Fatalf("ByPlayer(playerA) not most-recent-first: %v", handIDs(byPlayerA))
	}

	limited := store.ByPlayer(playerA, 2)
	if len(limited) != 2 {
		t.Fatalf("ByPlayer(playerA, 2) len = %d, want 2", len(limited))
	}
	if limited[0].HandID != "hand-3" || limited[1].HandID != "hand-2" {
		t.Fatalf("ByPlayer(playerA, 2) = %v", handIDs(limited))
	}

	byPlayerB := store.ByPlayer(playerB, 0)
	if len(byPlayerB) != 1 || byPlayerB[0].HandID != "hand-1" {
		t.Fatalf("ByPlayer(playerB) = %v", handIDs(byPlayerB))
	}
}

func handIDs(recs []history.HandRecord) []string {
	ids := make([]string, len(recs))
	for i, r := range recs {
		ids[i] = r.HandID
	}
	return ids
}
