package history

import (
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

func makeRecord(handID, tableID string, startedAt time.Time, playerIDs ...string) HandRecord {
	seats := make([]SeatInfo, 0, len(playerIDs))
	for i, pid := range playerIDs {
		seats = append(seats, SeatInfo{SeatID: i, PlayerID: pid, StartStack: 1000})
	}
	return HandRecord{
		HandID:    handID,
		TableID:   tableID,
		StartedAt: startedAt,
		Seats:     seats,
		Results:   map[int]string{},
	}
}

func TestStoreSaveAndGet(t *testing.T) {
	s := NewMemStore()
	rec := makeRecord("h1", "t1", time.Now(), "alice", "bob")

	if err := s.Save(rec); err != nil {
		t.Fatalf("unexpected save error: %v", err)
	}

	got, ok := s.Get("h1")
	if !ok {
		t.Fatal("expected to find saved record")
	}
	if got.HandID != "h1" {
		t.Fatalf("unexpected record: %+v", got)
	}

	if _, ok := s.Get("missing"); ok {
		t.Fatal("expected miss for unknown hand id")
	}
}

func TestStoreByPlayerOrderingAndLimit(t *testing.T) {
	s := NewMemStore()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	_ = s.Save(makeRecord("h1", "t1", base, "alice"))
	_ = s.Save(makeRecord("h2", "t1", base.Add(time.Minute), "alice", "bob"))
	_ = s.Save(makeRecord("h3", "t1", base.Add(2*time.Minute), "bob"))
	_ = s.Save(makeRecord("h4", "t1", base.Add(3*time.Minute), "alice"))

	all := s.ByPlayer("alice", 0)
	if len(all) != 3 {
		t.Fatalf("expected 3 records for alice, got %d", len(all))
	}
	// Most-recent-first.
	wantOrder := []string{"h4", "h2", "h1"}
	for i, want := range wantOrder {
		if all[i].HandID != want {
			t.Fatalf("order mismatch at %d: want %s got %s", i, want, all[i].HandID)
		}
	}

	limited := s.ByPlayer("alice", 2)
	if len(limited) != 2 {
		t.Fatalf("expected limit=2 to yield 2 records, got %d", len(limited))
	}
	if limited[0].HandID != "h4" || limited[1].HandID != "h2" {
		t.Fatalf("unexpected limited order: %+v", limited)
	}

	none := s.ByPlayer("nobody", 0)
	if len(none) != 0 {
		t.Fatalf("expected no records for unknown player, got %d", len(none))
	}
}

func TestStoreSaveOverwritesWithoutDuplicatingOrder(t *testing.T) {
	s := NewMemStore()
	rec := makeRecord("h1", "t1", time.Now(), "alice")

	_ = s.Save(rec)
	rec.Awards = []engine.Award{{SeatID: 0, Amount: 42}}
	_ = s.Save(rec)

	got, _ := s.Get("h1")
	if len(got.Awards) != 1 || got.Awards[0].Amount != 42 {
		t.Fatalf("expected overwrite to update record, got %+v", got)
	}

	all := s.ByPlayer("alice", 0)
	if len(all) != 1 {
		t.Fatalf("expected re-save not to duplicate ordering entry, got %d entries", len(all))
	}
}
