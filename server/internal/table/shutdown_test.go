package table

import (
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// balance is a small helper reading a player's durable ledger balance.
func (h *harness) balance(playerID string) int64 {
	return int64(h.ledger.Balance(playerID))
}

func TestShutdownRefundsInFlightHandAndCashesOutStacks(t *testing.T) {
	// Arrange: two players buy in and a hand auto-starts (public table), then one
	// raises so real chips are committed to the pot.
	h := newHarnessCfg(t, Config{ID: "t1", MaxSeats: 6, SmallBlind: 10, BigBlind: 20},
		Deps{TurnTimeout: 60 * time.Second})
	start := int64(economy.StartingBalance)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)
	h.bet("alice", "raise", 40) // alice commits 40 to the pot mid-hand
	h.waitFor("alice", protocol.EvBetPlaced)

	// Act: drain the table (server shutdown path).
	h.tbl.Shutdown(2 * time.Second)

	// Assert: the loop exited, clients were told, and every chip was conserved -
	// the aborted hand's committed chips were refunded and each seat's full stack
	// was cashed out, so both balances return exactly to the starting balance.
	select {
	case <-h.tbl.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("table loop did not exit after Shutdown")
	}
	h.waitFor("alice", protocol.EvServerShutdown)
	if got := h.balance("alice"); got != start {
		t.Fatalf("alice balance = %d, want %d (in-flight chips must be refunded)", got, start)
	}
	if got := h.balance("bob"); got != start {
		t.Fatalf("bob balance = %d, want %d (in-flight chips must be refunded)", got, start)
	}
}

func TestShutdownCashesOutBetweenHandsStacks(t *testing.T) {
	// Arrange: a private room where no hand deals until the host starts one, so
	// seats hold their between-hands stacks with no pot in flight.
	h := newHarnessCfg(t, Config{ID: "t1", Visibility: Private, MaxSeats: 6, SmallBlind: 10, BigBlind: 20},
		Deps{TurnTimeout: 60 * time.Second})
	start := int64(economy.StartingBalance)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1500)

	// Act.
	h.tbl.Shutdown(2 * time.Second)

	// Assert: both seats' stacks were cashed out back to the durable balance.
	select {
	case <-h.tbl.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("table loop did not exit after Shutdown")
	}
	if got := h.balance("alice"); got != start {
		t.Fatalf("alice balance = %d, want %d", got, start)
	}
	if got := h.balance("bob"); got != start {
		t.Fatalf("bob balance = %d, want %d", got, start)
	}
}

func TestSubmitReturnsFalseAfterShutdown(t *testing.T) {
	// Arrange.
	h := newHarnessCfg(t, Config{ID: "t1", Visibility: Private, MaxSeats: 6, SmallBlind: 10, BigBlind: 20},
		Deps{TurnTimeout: 60 * time.Second})
	h.tbl.Shutdown(2 * time.Second)
	<-h.tbl.Done()

	// Act + Assert: a late Submit must report the table is gone, not block forever.
	ok := h.tbl.Submit(Command{PlayerID: "late", Reply: h.chanFor("late"), Msg: tableIDMsg(protocol.CmdJoinTable)})
	if ok {
		t.Fatal("Submit must return false once the loop has stopped")
	}
}

func TestShutdownDoesNotCashOutTournamentChips(t *testing.T) {
	// Tournament chips are not ledger balances (the buy-in was collected by the
	// tourney manager), so draining a tournament table must NOT credit them back.
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	ledger := economy.NewLedger(economy.NewMemoryStore(), clock.Now)
	before := int64(ledger.Balance("p0"))

	tbl := New(Config{
		ID: "tt", Visibility: Private, MaxSeats: 3, AutoStart: true, SmallBlind: 10, BigBlind: 20,
		Tournament: &TourneyRules{StartingStack: 1500, NoRebuy: true, Seats: []TourneySeat{
			{Seat: 0, PlayerID: "p0"}, {Seat: 1, PlayerID: "p1"},
		}},
	}, Deps{Ledger: ledger, Now: clock.Now, Clock: clock, TurnTimeout: 60 * time.Second})

	tbl.Shutdown(2 * time.Second)
	<-tbl.Done()

	if got := int64(ledger.Balance("p0")); got != before {
		t.Fatalf("tournament seat balance changed on drain: got %d, want %d", got, before)
	}
}
