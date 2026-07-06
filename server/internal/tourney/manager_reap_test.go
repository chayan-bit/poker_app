package tourney

import (
	"sync"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
)

// clockFixture builds a manager over a mutable clock so reaping TTLs can be
// tripped deterministically.
func clockFixture(t *testing.T) (*Manager, *economy.Ledger, func(time.Duration)) {
	t.Helper()
	var mu sync.Mutex
	cur := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	now := func() time.Time { mu.Lock(); defer mu.Unlock(); return cur }
	advance := func(d time.Duration) { mu.Lock(); cur = cur.Add(d); mu.Unlock() }

	ledger := economy.NewLedger(economy.NewMemoryStore(), now)
	factory := &fakeFactory{now: now, ledger: ledger}
	m := NewManager(ledger, factory)
	m.now = now
	return m, ledger, advance
}

func TestReapRefundsNeverFilledSNG(t *testing.T) {
	m, ledger, advance := clockFixture(t)
	start := ledger.Balance("a")

	sngID, _, _ := m.Create(DefaultConfig("t", 3, 100))
	if err := m.Register(sngID, "a"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := ledger.Balance("a"); got != start-100 {
		t.Fatalf("buy-in not debited: balance %d", got)
	}

	// Age the SNG past the register TTL, then trigger a reap via List.
	advance(registerTTL + time.Minute)
	if list := m.List(); len(list) != 0 {
		t.Fatalf("expired SNG should be reaped from the listing, got %d", len(list))
	}
	if got := ledger.Balance("a"); got != start {
		t.Fatalf("reaped never-filled SNG must refund the buy-in: balance %d, want %d", got, start)
	}
	if err := m.Register(sngID, "b"); err != ErrNotFound {
		t.Fatalf("register into reaped SNG err = %v, want ErrNotFound", err)
	}
}

func TestShutdownRefundsOpenRegistrations(t *testing.T) {
	m, ledger, _ := clockFixture(t)
	start := ledger.Balance("a")

	sngID, _, _ := m.Create(DefaultConfig("t", 3, 100))
	if err := m.Register(sngID, "a"); err != nil {
		t.Fatalf("register: %v", err)
	}

	m.Shutdown()

	if got := ledger.Balance("a"); got != start {
		t.Fatalf("shutdown must refund open registrations: balance %d, want %d", got, start)
	}
	if err := m.Register(sngID, "b"); err != ErrNotFound {
		t.Fatalf("register after shutdown err = %v, want ErrNotFound", err)
	}
}
