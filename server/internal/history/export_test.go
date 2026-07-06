package history

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

func fixedRecord() HandRecord {
	return HandRecord{
		HandID:     "hand-42",
		TableID:    "table-9",
		StartedAt:  time.Date(2026, 3, 4, 18, 30, 0, 0, time.UTC),
		ButtonSeat: 1,
		Blinds:     [2]engine.Chips{5, 10},
		Commitment: "commit-hex-value",
		SeedHex:    "seed-hex-value",
		Seats: []SeatInfo{
			{SeatID: 0, PlayerID: "alice", StartStack: 1000},
			{SeatID: 1, PlayerID: "bob", StartStack: 1000},
		},
		Events: []Event{
			{Street: "preflop", SeatID: 0, Kind: "call", Amount: 10},
			{Street: "preflop", SeatID: 1, Kind: "check", Amount: 0},
			{Street: "flop", Kind: "street"},
			{Street: "flop", SeatID: 0, Kind: "bet", Amount: 20},
			{Street: "flop", SeatID: 1, Kind: "fold", Amount: 0},
		},
		Board: []string{"As", "Kd", "2c"},
		Awards: []engine.Award{
			{SeatID: 0, Amount: 40},
		},
		Results: map[int]string{
			0: "won 40 (uncontested)",
		},
	}
}

func TestExportTextContainsKeyLines(t *testing.T) {
	text := ExportText(fixedRecord())

	wantLines := []string{
		"PokerApp Hand #hand-42: Table 'table-9' (5/10)",
		"Hand started at 2026-03-04 18:30:00 UTC",
		"Table 'table-9' Seat #1 is the button",
		"Seat 0: alice (1000 in chips)",
		"Seat 1: bob (1000 in chips)",
		"*** FLOP *** [As Kd 2c]",
		"Seat 0: call 10",
		"Seat 1: check",
		"Seat 0: bet 20",
		"Seat 1: fold",
		"*** SUMMARY ***",
		"Board [As Kd 2c]",
		"Seat 0 collected 40 from pot",
		"Seat 0: won 40 (uncontested)",
		"Fairness commitment: commit-hex-value",
		"Fairness seed: seed-hex-value",
	}

	for _, want := range wantLines {
		if !strings.Contains(text, want) {
			t.Errorf("expected export text to contain %q\n--- got ---\n%s", want, text)
		}
	}
}

func TestExportTextIsDeterministic(t *testing.T) {
	rec := fixedRecord()
	first := ExportText(rec)
	second := ExportText(rec)
	if first != second {
		t.Fatal("expected ExportText to be deterministic given a fixed record")
	}
}

func TestExportJSONRoundtrip(t *testing.T) {
	rec := fixedRecord()

	data, err := ExportJSON(rec)
	if err != nil {
		t.Fatalf("unexpected export error: %v", err)
	}

	var got HandRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}

	if got.HandID != rec.HandID || got.TableID != rec.TableID {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", got, rec)
	}
	if !got.StartedAt.Equal(rec.StartedAt) {
		t.Fatalf("roundtrip StartedAt mismatch: %v vs %v", got.StartedAt, rec.StartedAt)
	}
	if got.Commitment != rec.Commitment || got.SeedHex != rec.SeedHex {
		t.Fatalf("roundtrip fairness field mismatch")
	}
	if len(got.Events) != len(rec.Events) {
		t.Fatalf("roundtrip events length mismatch: %d vs %d", len(got.Events), len(rec.Events))
	}
	if len(got.Awards) != 1 || got.Awards[0].Amount != 40 {
		t.Fatalf("roundtrip awards mismatch: %+v", got.Awards)
	}
	if got.Results[0] != "won 40 (uncontested)" {
		t.Fatalf("roundtrip results mismatch: %+v", got.Results)
	}
}
