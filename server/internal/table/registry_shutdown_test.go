package table

import (
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
)

func newTestRegistry() *Registry {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	return NewRegistryWithDeps(Deps{
		Ledger:      economy.NewLedger(economy.NewMemoryStore(), clock.Now),
		Now:         clock.Now,
		Clock:       clock,
		IdleTimeout: time.Hour,
		TurnTimeout: time.Hour,
	})
}

func TestRegistryPerCreatorCapBlocksAndReleases(t *testing.T) {
	reg := newTestRegistry()
	reg.SetMaxPerCreator(2)
	defer reg.DrainAll(time.Second)

	if !reg.CanCreateFor("h1") {
		t.Fatal("fresh creator should be under cap")
	}
	reg.Create(Config{ID: "a", HostPlayerID: "h1"})
	reg.Create(Config{ID: "b", HostPlayerID: "h1"})

	if reg.CanCreateFor("h1") {
		t.Fatal("creator at the cap must be blocked")
	}
	if !reg.CanCreateFor("h2") {
		t.Fatal("a different creator must be unaffected")
	}
	if !reg.CanCreateFor("") {
		t.Fatal("unowned (empty) creator is never capped")
	}

	reg.Remove("a")
	if !reg.CanCreateFor("h1") {
		t.Fatal("removing a table must free a creator slot")
	}
}

func TestDrainAllRemovesEveryTableAndExits(t *testing.T) {
	reg := newTestRegistry()
	a := reg.Create(Config{ID: "a", HostPlayerID: "h1"})
	b := reg.Create(Config{ID: "b"})

	reg.DrainAll(2 * time.Second)

	if _, ok := reg.Get("a"); ok {
		t.Fatal("table a should be removed after DrainAll")
	}
	if _, ok := reg.Get("b"); ok {
		t.Fatal("table b should be removed after DrainAll")
	}
	select {
	case <-a.Done():
	default:
		t.Fatal("table a loop should have exited")
	}
	select {
	case <-b.Done():
	default:
		t.Fatal("table b loop should have exited")
	}
}
