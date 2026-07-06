package history

import (
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

func scriptedRecorder() *Recorder {
	seats := []SeatInfo{
		{SeatID: 0, PlayerID: "alice", StartStack: 1000},
		{SeatID: 1, PlayerID: "bob", StartStack: 1000},
		{SeatID: 2, PlayerID: "carol", StartStack: 1000},
	}
	startedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	r := NewRecorder("hand-1", "table-1", startedAt, 0, [2]engine.Chips{5, 10}, "commit-abc", seats)

	r.OnAction("preflop", 1, "call", 10)
	r.OnAction("preflop", 2, "raise", 30)
	r.OnAction("preflop", 0, "fold", 0)
	r.OnAction("preflop", 1, "call", 30)

	r.OnStreet("flop", []string{"As", "Kd", "2c"})
	r.OnAction("flop", 1, "check", 0)
	r.OnAction("flop", 2, "bet", 40)
	r.OnAction("flop", 1, "call", 40)

	r.OnStreet("turn", []string{"7h"})
	r.OnAction("turn", 1, "check", 0)
	r.OnAction("turn", 2, "check", 0)

	r.OnStreet("river", []string{"9s"})
	r.OnAction("river", 1, "check", 0)
	r.OnAction("river", 2, "check", 0)

	r.OnShowdown(
		[]engine.Award{{SeatID: 2, Amount: 150}},
		map[int]string{2: "won 150 with Pair of Aces"},
	)
	r.OnReveal("deadbeef")

	return r
}

func TestRecorderBuildsCoherentRecord(t *testing.T) {
	rec := scriptedRecorder().Finish()

	if rec.HandID != "hand-1" || rec.TableID != "table-1" {
		t.Fatalf("unexpected hand/table id: %+v", rec)
	}
	if rec.ButtonSeat != 0 {
		t.Fatalf("expected button seat 0, got %d", rec.ButtonSeat)
	}
	if rec.Blinds != [2]engine.Chips{5, 10} {
		t.Fatalf("unexpected blinds: %+v", rec.Blinds)
	}
	if rec.Commitment != "commit-abc" || rec.SeedHex != "deadbeef" {
		t.Fatalf("unexpected fairness fields: commitment=%s seed=%s", rec.Commitment, rec.SeedHex)
	}
	if len(rec.Seats) != 3 {
		t.Fatalf("expected 3 seats, got %d", len(rec.Seats))
	}

	wantBoard := []string{"As", "Kd", "2c", "7h", "9s"}
	if len(rec.Board) != len(wantBoard) {
		t.Fatalf("expected board %v, got %v", wantBoard, rec.Board)
	}
	for i, c := range wantBoard {
		if rec.Board[i] != c {
			t.Fatalf("board mismatch at %d: want %s got %s", i, c, rec.Board[i])
		}
	}

	// 3 street markers (flop/turn/river; preflop has none) + 11 action events.
	if len(rec.Events) != 14 {
		t.Fatalf("expected 14 events, got %d", len(rec.Events))
	}

	if len(rec.Awards) != 1 || rec.Awards[0].SeatID != 2 || rec.Awards[0].Amount != 150 {
		t.Fatalf("unexpected awards: %+v", rec.Awards)
	}
	if desc := rec.Results[2]; desc != "won 150 with Pair of Aces" {
		t.Fatalf("unexpected result desc: %s", desc)
	}
}

func TestRecorderSeatsAreSnapshotted(t *testing.T) {
	seats := []SeatInfo{{SeatID: 0, PlayerID: "alice", StartStack: 500}}
	r := NewRecorder("h", "t", time.Now(), 0, [2]engine.Chips{1, 2}, "c", seats)

	// Mutating the original slice after construction must not affect the
	// recorder's internal copy.
	seats[0].PlayerID = "mutated"

	rec := r.Finish()
	if rec.Seats[0].PlayerID != "alice" {
		t.Fatalf("expected recorder to snapshot seats, got %s", rec.Seats[0].PlayerID)
	}
}
