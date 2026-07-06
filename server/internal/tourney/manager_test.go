package tourney

import (
	"sync"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// fakeFactory records CreateTourney calls and builds a real table.Table with a
// fake clock + in-memory ledger so integration flows are deterministic.
type fakeFactory struct {
	mu      sync.Mutex
	clock   table.Clock
	now     func() time.Time
	ledger  *economy.Ledger
	created []createdTable
}

type createdTable struct {
	cfg      table.Config
	callback table.OnHandComplete
	tbl      *table.Table
}

func (f *fakeFactory) CreateTourney(cfg table.Config, onComplete table.OnHandComplete) *table.Table {
	f.mu.Lock()
	defer f.mu.Unlock()
	tbl := table.New(cfg, table.Deps{
		Ledger:         f.ledger,
		Now:            f.now,
		Clock:          f.clock,
		OnHandComplete: onComplete,
	})
	f.created = append(f.created, createdTable{cfg: cfg, callback: onComplete, tbl: tbl})
	return tbl
}

func (f *fakeFactory) last() createdTable {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.created[len(f.created)-1]
}

func (f *fakeFactory) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.created)
}

func newManagerFixture(t *testing.T) (*Manager, *fakeFactory, *economy.Ledger) {
	t.Helper()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	ledger := economy.NewLedger(economy.NewMemoryStore(), func() time.Time { return now })
	factory := &fakeFactory{now: func() time.Time { return now }, ledger: ledger}
	m := NewManager(ledger, factory)
	m.now = func() time.Time { return now }
	return m, factory, ledger
}

func TestCreateValidatesAndAllocatesIDs(t *testing.T) {
	m, _, _ := newManagerFixture(t)
	sngID, tableID, err := m.Create(DefaultConfig("Friday SNG", 3, 100))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if sngID == "" || tableID == "" || sngID == tableID {
		t.Fatalf("expected distinct non-empty ids, got sng=%q table=%q", sngID, tableID)
	}
}

func TestCreateRejectsBadConfig(t *testing.T) {
	m, _, _ := newManagerFixture(t)
	if _, _, err := m.Create(DefaultConfig("x", 1, 100)); err != ErrBadSeats {
		t.Fatalf("seats=1 err = %v, want ErrBadSeats", err)
	}
	if _, _, err := m.Create(DefaultConfig("x", 3, 0)); err != ErrBadBuyIn {
		t.Fatalf("buyIn=0 err = %v, want ErrBadBuyIn", err)
	}
}

func TestRegisterFillsAndAutoStartsWithTournamentConfig(t *testing.T) {
	m, factory, _ := newManagerFixture(t)
	sngID, tableID, _ := m.Create(DefaultConfig("t", 3, 100))

	if err := m.Register(sngID, "a"); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := m.Register(sngID, "b"); err != nil {
		t.Fatalf("register b: %v", err)
	}
	if factory.count() != 0 {
		t.Fatalf("table must not be created before the SNG fills")
	}
	if err := m.Register(sngID, "c"); err != nil {
		t.Fatalf("register c: %v", err)
	}
	if factory.count() != 1 {
		t.Fatalf("table must be created on the final registration")
	}

	ct := factory.last()
	if ct.cfg.ID != tableID {
		t.Fatalf("table created with id %q, want reserved %q", ct.cfg.ID, tableID)
	}
	if ct.cfg.Tournament == nil {
		t.Fatalf("table must be created in tournament mode")
	}
	if ct.cfg.Tournament.StartingStack != DefaultStartingStack {
		t.Fatalf("starting stack = %d, want %d", ct.cfg.Tournament.StartingStack, DefaultStartingStack)
	}
	if !ct.cfg.Tournament.NoRebuy {
		t.Fatalf("tournament tables must forbid rebuys")
	}
	if ct.cfg.SmallBlind != 10 || ct.cfg.BigBlind != 20 {
		t.Fatalf("table must open at level-1 blinds 10/20, got %d/%d", ct.cfg.SmallBlind, ct.cfg.BigBlind)
	}
	if len(ct.cfg.Tournament.Seats) != 3 {
		t.Fatalf("expected 3 seated players, got %d", len(ct.cfg.Tournament.Seats))
	}
}

func TestRegisterErrors(t *testing.T) {
	m, _, _ := newManagerFixture(t)
	sngID, _, _ := m.Create(DefaultConfig("t", 2, 100))

	if err := m.Register("nope", "a"); err != ErrNotFound {
		t.Fatalf("unknown sng err = %v, want ErrNotFound", err)
	}
	if err := m.Register(sngID, "a"); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := m.Register(sngID, "a"); err != ErrAlreadyRegistered {
		t.Fatalf("double register err = %v, want ErrAlreadyRegistered", err)
	}
	// Second unique registration fills the 2-seat SNG; further registration is closed.
	if err := m.Register(sngID, "b"); err != nil {
		t.Fatalf("register b: %v", err)
	}
	if err := m.Register(sngID, "d"); err != ErrFull {
		t.Fatalf("register after full err = %v, want ErrFull", err)
	}
}

func TestRegisterInsufficientFunds(t *testing.T) {
	m, _, ledger := newManagerFixture(t)
	// Buy-in above the starting balance (10_000) fails at the ledger.
	sngID, _, _ := m.Create(DefaultConfig("t", 2, engine.Chips(economy.StartingBalance+1)))
	if err := m.Register(sngID, "broke"); err != economy.ErrInsufficientFunds {
		t.Fatalf("register err = %v, want ErrInsufficientFunds", err)
	}
	if ledger.Balance("broke") != economy.StartingBalance {
		t.Fatalf("failed registration must not debit the ledger")
	}
}

func TestListShowsOpenSNGsOnly(t *testing.T) {
	m, _, _ := newManagerFixture(t)
	openID, _, _ := m.Create(DefaultConfig("open", 3, 100))
	fullID, _, _ := m.Create(DefaultConfig("full", 2, 100))
	_ = m.Register(fullID, "a")
	_ = m.Register(fullID, "b") // fills -> Running -> hidden from the list

	list := m.List()
	if len(list) != 1 || list[0].SngID != openID {
		t.Fatalf("list should show only the open SNG, got %+v", list)
	}
	if list[0].Registered != 0 || list[0].Seats != 3 || list[0].BuyIn != 100 {
		t.Fatalf("unexpected view fields: %+v", list[0])
	}
	_ = fullID
}
