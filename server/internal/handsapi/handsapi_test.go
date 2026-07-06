package handsapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/history"
)

func fakeAuth(playerID string, fail bool) AuthFunc {
	return func(r *http.Request) (string, error) {
		if fail {
			return "", errors.New("unauthorized")
		}
		return playerID, nil
	}
}

func sampleRecord(handID, tableID, playerID string, when time.Time, potWon engine.Chips) history.HandRecord {
	return history.HandRecord{
		HandID:     handID,
		TableID:    tableID,
		StartedAt:  when,
		ButtonSeat: 0,
		Blinds:     [2]engine.Chips{1, 2},
		Commitment: "deadbeef",
		SeedHex:    "cafebabe",
		Seats: []history.SeatInfo{
			{SeatID: 0, PlayerID: playerID, StartStack: 100},
			{SeatID: 1, PlayerID: "opponent", StartStack: 100},
		},
		Board:   []string{"As", "Kd", "2c"},
		Awards:  []engine.Award{{SeatID: 0, Amount: potWon}},
		Results: map[int]string{0: "won with Two Pair"},
	}
}

func newStoreWithRecords(recs ...history.HandRecord) history.Store {
	s := history.NewMemStore()
	for _, r := range recs {
		_ = s.Save(r)
	}
	return s
}

func TestGetHand_HappyPath(t *testing.T) {
	rec := sampleRecord("h1", "t1", "p1", time.Now(), 30)
	store := newStoreWithRecords(rec)
	h := New(store, fakeAuth("p1", false))
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/hands/h1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got history.HandRecord
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.HandID != "h1" || got.Commitment != "deadbeef" || got.SeedHex != "cafebabe" {
		t.Fatalf("unexpected record: %+v", got)
	}
}

func TestGetHand_Unauthorized(t *testing.T) {
	store := newStoreWithRecords(sampleRecord("h1", "t1", "p1", time.Now(), 30))
	h := New(store, fakeAuth("p1", true))
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/hands/h1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestGetHand_NotFound(t *testing.T) {
	store := newStoreWithRecords()
	h := New(store, fakeAuth("p1", false))
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/hands/missing", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestGetHandText(t *testing.T) {
	rec := sampleRecord("h1", "t1", "p1", time.Now(), 30)
	store := newStoreWithRecords(rec)
	h := New(store, fakeAuth("p1", false))
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/hands/h1/text", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", ct)
	}
	if !strings.Contains(w.Body.String(), "Hand #h1") {
		t.Fatalf("body missing hand id: %s", w.Body.String())
	}
}

func TestGetHandText_NotFound(t *testing.T) {
	store := newStoreWithRecords()
	h := New(store, fakeAuth("p1", false))
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/hands/missing/text", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestListMyHands_Unauthorized(t *testing.T) {
	store := newStoreWithRecords()
	h := New(store, fakeAuth("p1", true))
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/players/me/hands", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestListMyHands_PotWonAggregation(t *testing.T) {
	now := time.Now()
	recs := []history.HandRecord{
		sampleRecord("h1", "t1", "p1", now.Add(-2*time.Hour), 50),
		sampleRecord("h2", "t1", "p1", now.Add(-1*time.Hour), 0),
	}
	store := newStoreWithRecords(recs...)
	h := New(store, fakeAuth("p1", false))
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/players/me/hands", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got []handSummary
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	// most-recent-first: h2 (potWon 0) then h1 (potWon 50)
	if got[0].HandID != "h2" || got[0].PotWon != 0 {
		t.Fatalf("got[0] = %+v", got[0])
	}
	if got[1].HandID != "h1" || got[1].PotWon != 50 {
		t.Fatalf("got[1] = %+v", got[1])
	}
}

func TestListMyHands_LimitCapping(t *testing.T) {
	now := time.Now()
	var recs []history.HandRecord
	for i := 0; i < 150; i++ {
		recs = append(recs, sampleRecord(
			"h"+string(rune('a'+i%26))+string(rune('0'+i/26)),
			"t1", "p1", now.Add(time.Duration(-i)*time.Minute), 1))
	}
	store := newStoreWithRecords(recs...)
	h := New(store, fakeAuth("p1", false))
	mux := http.NewServeMux()
	h.Register(mux)

	cases := []struct {
		query string
		want  int
	}{
		{"", defaultHandsLimit},
		{"?limit=5", 5},
		{"?limit=1000", maxHandsLimit},
		{"?limit=-1", defaultHandsLimit},
		{"?limit=abc", defaultHandsLimit},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/players/me/hands"+tc.query, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var got []handSummary
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal (%s): %v", tc.query, err)
		}
		if len(got) != tc.want {
			t.Fatalf("query %q: len = %d, want %d", tc.query, len(got), tc.want)
		}
	}
}

func TestParseLimit(t *testing.T) {
	cases := map[string]int{
		"":     defaultHandsLimit,
		"0":    defaultHandsLimit,
		"-5":   defaultHandsLimit,
		"20":   20,
		"100":  100,
		"101":  maxHandsLimit,
		"9999": maxHandsLimit,
		"abc":  defaultHandsLimit,
	}
	for in, want := range cases {
		if got := parseLimit(in); got != want {
			t.Errorf("parseLimit(%q) = %d, want %d", in, got, want)
		}
	}
}
