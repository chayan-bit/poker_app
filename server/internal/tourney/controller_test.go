package tourney

import (
	"errors"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// recordingLedger records credits so payout tests can assert them without a
// real economy store.
type recordingLedger struct {
	credited map[string]engine.Chips
	balance  engine.Chips // pretend every player can always afford the buy-in
}

func newRecordingLedger() *recordingLedger {
	return &recordingLedger{credited: map[string]engine.Chips{}, balance: 1_000_000}
}

func (l *recordingLedger) BuyIn(_ string, amt engine.Chips) error {
	if amt > l.balance {
		return errTestInsufficient
	}
	return nil
}
func (l *recordingLedger) Credit(playerID string, amt engine.Chips) {
	l.credited[playerID] += amt
}

// controllerFixture builds a Running SNG with a fixed start time and injected
// clock, ready to receive onHandComplete calls directly.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newRunningSNG(t *testing.T, cfg SNGConfig, players []string, led Ledger) (*SNG, *clock) {
	t.Helper()
	clk := &clock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := &SNG{
		ID:         "sng-1",
		TableID:    "tbl-1",
		Cfg:        cfg,
		now:        clk.Now,
		status:     Running,
		registered: players,
		prizePool:  cfg.BuyIn * engine.Chips(len(players)),
		startTime:  clk.now,
		ledger:     led,
	}
	return s, clk
}

func stand(seat int, playerID string, stack, start engine.Chips) table.SeatResult {
	return table.SeatResult{Seat: seat, PlayerID: playerID, Stack: stack, StartStack: start}
}

// --- blind schedule ---

func TestBlindScheduleAdvancesOnlyAtHandBoundariesByElapsedTime(t *testing.T) {
	cfg := DefaultConfig("t", 3, 100)
	s, clk := newRunningSNG(t, cfg, []string{"a", "b", "c"}, newRecordingLedger())

	// Hand 1 completes still within level 1: no change.
	d := s.onHandComplete([]table.SeatResult{stand(0, "a", 1500, 1500), stand(1, "b", 1500, 1500), stand(2, "c", 1500, 1500)})
	if d.BlindsChanged {
		t.Fatalf("blinds must not change inside level 1")
	}
	if d.Level != 1 || d.SmallBlind != 10 || d.BigBlind != 20 {
		t.Fatalf("level 1 blinds = %d/%d level %d, want 10/20 level 1", d.SmallBlind, d.BigBlind, d.Level)
	}

	// Advance past level 1's duration; the NEXT completion raises to level 2.
	clk.now = clk.now.Add(DefaultLevelDuration + time.Second)
	d = s.onHandComplete([]table.SeatResult{stand(0, "a", 1500, 1500), stand(1, "b", 1500, 1500), stand(2, "c", 1500, 1500)})
	if !d.BlindsChanged {
		t.Fatalf("blinds must raise after the level duration elapses")
	}
	if d.Level != 2 || d.SmallBlind != 15 || d.BigBlind != 30 {
		t.Fatalf("level 2 blinds = %d/%d level %d, want 15/30 level 2", d.SmallBlind, d.BigBlind, d.Level)
	}

	// Same level again on the next hand: no repeated blinds_up.
	d = s.onHandComplete([]table.SeatResult{stand(0, "a", 1500, 1500), stand(1, "b", 1500, 1500), stand(2, "c", 1500, 1500)})
	if d.BlindsChanged {
		t.Fatalf("blinds must not re-announce within the same level")
	}
}

func TestBlindScheduleCapsAtFinalLevel(t *testing.T) {
	cfg := DefaultConfig("t", 3, 100)
	s, clk := newRunningSNG(t, cfg, []string{"a", "b", "c"}, newRecordingLedger())
	clk.now = clk.now.Add(1000 * time.Hour) // way past the schedule
	d := s.onHandComplete([]table.SeatResult{stand(0, "a", 1500, 1500), stand(1, "b", 1500, 1500)})
	last := cfg.BlindLevels[len(cfg.BlindLevels)-1]
	if d.SmallBlind != last.SmallBlind || d.BigBlind != last.BigBlind {
		t.Fatalf("blinds must cap at the final level %d/%d, got %d/%d", last.SmallBlind, last.BigBlind, d.SmallBlind, d.BigBlind)
	}
}

// --- eliminations in order + completion + payout ---

func TestSequentialEliminationsAssignPlacesAndPayout(t *testing.T) {
	cfg := DefaultConfig("t", 3, 100) // pool 300, winner-take-all
	led := newRecordingLedger()
	s, _ := newRunningSNG(t, cfg, []string{"a", "b", "c"}, led)

	// Hand 1: c busts (had 500 at hand start) -> place 3.
	d := s.onHandComplete([]table.SeatResult{stand(0, "a", 2500, 2000), stand(1, "b", 2000, 2000), stand(2, "c", 0, 500)})
	if len(d.Eliminations) != 1 || d.Eliminations[0].PlayerID != "c" || d.Eliminations[0].Place != 3 {
		t.Fatalf("first bust should be c in place 3, got %+v", d.Eliminations)
	}
	if d.Done {
		t.Fatalf("tournament not over with 2 players left")
	}

	// Hand 2: b busts -> place 2; a wins -> place 1, done.
	d = s.onHandComplete([]table.SeatResult{stand(0, "a", 4500, 2500), stand(1, "b", 0, 2000)})
	if len(d.Eliminations) != 1 || d.Eliminations[0].PlayerID != "b" || d.Eliminations[0].Place != 2 {
		t.Fatalf("second bust should be b in place 2, got %+v", d.Eliminations)
	}
	if !d.Done || d.Result == nil {
		t.Fatalf("tournament should be done with a result")
	}
	// Winner-take-all: a gets 300, others 0.
	if led.credited["a"] != 300 || led.credited["b"] != 0 || led.credited["c"] != 0 {
		t.Fatalf("payout = %+v, want a:300", led.credited)
	}
	assertPlaces(t, d.Result.Places, map[int]string{1: "a", 2: "b", 3: "c"})
}

func TestSimultaneousBustBiggerStartStackFinishesHigher(t *testing.T) {
	// 6-seat payout [65,35], pool 600. b and c bust the same hand; b started the
	// hand with more chips, so b finishes 2nd and c 3rd.
	cfg := SNGConfig{Name: "t", Seats: 3, BuyIn: 200, StartingStack: 1500,
		BlindLevels: DefaultBlindSchedule(), PayoutPct: []int{65, 35}}
	led := newRecordingLedger()
	s, _ := newRunningSNG(t, cfg, []string{"a", "b", "c"}, led)

	d := s.onHandComplete([]table.SeatResult{
		stand(0, "a", 4500, 1500),
		stand(1, "b", 0, 1000), // bigger start stack among the busted
		stand(2, "c", 0, 500),
	})
	if !d.Done {
		t.Fatalf("one player left, tournament should be done")
	}
	places := placeMap(d.Result.Places)
	if places["b"] != 2 || places["c"] != 3 || places["a"] != 1 {
		t.Fatalf("places = %+v, want a:1 b:2 c:3", places)
	}
	// pool 600: 1st 65% = 390, 2nd 35% = 210, 3rd 0.
	if led.credited["a"] != 390 || led.credited["b"] != 210 || led.credited["c"] != 0 {
		t.Fatalf("payout = %+v, want a:390 b:210 c:0", led.credited)
	}
}

func TestSimultaneousBustIdenticalStacksSplitCombinedPrize(t *testing.T) {
	// b and c bust with IDENTICAL start stacks -> genuine tie for places 2 and 3.
	// They pool the prizes for places 2 and 3 and split evenly; the odd chip goes
	// to the earlier (higher-placed) finisher.
	// pool 300 with payout [50,30,20] -> place1 150, place2 90, place3 60.
	// tie pools 90+60 = 150, split -> 75 each (even, no remainder).
	cfg := SNGConfig{Name: "t", Seats: 3, BuyIn: 100, StartingStack: 1500,
		BlindLevels: DefaultBlindSchedule(), PayoutPct: []int{50, 30, 20}}
	led := newRecordingLedger()
	s, _ := newRunningSNG(t, cfg, []string{"a", "b", "c"}, led)

	d := s.onHandComplete([]table.SeatResult{
		stand(0, "a", 4500, 1500),
		stand(1, "b", 0, 800),
		stand(2, "c", 0, 800),
	})
	if !d.Done {
		t.Fatalf("tournament should be done")
	}
	if led.credited["a"] != 150 || led.credited["b"] != 75 || led.credited["c"] != 75 {
		t.Fatalf("tie split payout = %+v, want a:150 b:75 c:75", led.credited)
	}
	// Total paid equals the pool.
	if led.credited["a"]+led.credited["b"]+led.credited["c"] != 300 {
		t.Fatalf("total payout must equal pool 300")
	}
}

func TestTieSplitRemainderToEarlierFinisher(t *testing.T) {
	// pool 103, payout [60,40] (only two paid places): prize1 = 62 (61 + rounding
	// remainder), prize2 = 41. b and c tie for places 2 and 3; place 3 is unpaid,
	// so they pool prize2 (41) and split it: 20 each with 1 chip left over, which
	// goes to the earlier (higher-placed, lower-seat) finisher b.
	cfg := SNGConfig{Name: "t", Seats: 3, BuyIn: 0, StartingStack: 1500,
		BlindLevels: DefaultBlindSchedule(), PayoutPct: []int{60, 40}}
	led := newRecordingLedger()
	s, _ := newRunningSNG(t, cfg, []string{"a", "b", "c"}, led)
	s.prizePool = 103
	d := s.onHandComplete([]table.SeatResult{
		stand(0, "a", 4500, 1500),
		stand(1, "b", 0, 800),
		stand(2, "c", 0, 800),
	})
	if !d.Done {
		t.Fatalf("tournament should be done")
	}
	// place2 prize = 41, tie for places 2 and 3 (place3 unpaid = 0) -> pool 41.
	// split 41 across 2 -> 20 each with remainder 1 to the earlier finisher.
	// Earlier finisher (place 2) is the lower-seat tie member = b (seat 1).
	if led.credited["b"] != 21 || led.credited["c"] != 20 {
		t.Fatalf("remainder must go to earlier finisher: b=%d c=%d, want b:21 c:20", led.credited["b"], led.credited["c"])
	}
}

// --- payout table defaults ---

func TestDefaultPayoutPctByFieldSize(t *testing.T) {
	cases := []struct {
		n    int
		want []int
	}{
		{2, []int{100}}, {3, []int{100}}, {4, []int{100}},
		{5, []int{65, 35}}, {6, []int{65, 35}},
		{7, []int{50, 30, 20}}, {8, []int{50, 30, 20}}, {9, []int{50, 30, 20}},
	}
	for _, c := range cases {
		got := DefaultPayoutPct(c.n)
		if len(got) != len(c.want) {
			t.Fatalf("n=%d payout %v, want %v", c.n, got, c.want)
		}
		sum := 0
		for i, p := range got {
			if p != c.want[i] {
				t.Fatalf("n=%d payout %v, want %v", c.n, got, c.want)
			}
			sum += p
		}
		if sum != 100 {
			t.Fatalf("n=%d payout must sum to 100, got %d", c.n, sum)
		}
	}
}

func TestComputePrizesRemainderToFirst(t *testing.T) {
	// pool 100, [50,30,20] divides cleanly.
	p := computePrizes(100, []int{50, 30, 20})
	if p[0] != 50 || p[1] != 30 || p[2] != 20 {
		t.Fatalf("clean split = %v", p)
	}
	// pool 101 -> 50/30/20 = 100, remainder 1 to first.
	p = computePrizes(101, []int{50, 30, 20})
	if p[0] != 51 || p[1] != 30 || p[2] != 20 {
		t.Fatalf("remainder split = %v, want [51 30 20]", p)
	}
	var sum engine.Chips
	for _, v := range p {
		sum += v
	}
	if sum != 101 {
		t.Fatalf("prizes must sum to pool 101, got %d", sum)
	}
}

// --- helpers ---

var errTestInsufficient = errors.New("test: insufficient")

func placeMap(places []protocol.TourneyPlace) map[string]int {
	m := map[string]int{}
	for _, p := range places {
		m[p.PlayerID] = p.Place
	}
	return m
}

func assertPlaces(t *testing.T, places []protocol.TourneyPlace, want map[int]string) {
	t.Helper()
	got := map[int]string{}
	for _, p := range places {
		got[p.Place] = p.PlayerID
	}
	for place, player := range want {
		if got[place] != player {
			t.Fatalf("place %d = %q, want %q (all=%+v)", place, got[place], player, places)
		}
	}
}
