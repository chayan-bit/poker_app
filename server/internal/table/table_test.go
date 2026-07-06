package table

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/history"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// ---- deterministic fake clock + timer ----

type fakeTimer struct {
	mu    sync.Mutex
	ch    chan time.Time
	armed bool
}

func (f *fakeTimer) C() <-chan time.Time { return f.ch }
func (f *fakeTimer) Reset(time.Duration)  { f.mu.Lock(); f.armed = true; f.mu.Unlock() }
func (f *fakeTimer) Stop()                { f.mu.Lock(); f.armed = false; f.mu.Unlock() }

// fire delivers a tick; the unbuffered send blocks until the loop consumes it,
// synchronizing the test with the resulting timeout handling.
func (f *fakeTimer) fire(now time.Time) { f.ch <- now }

type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	timer *fakeTimer
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer() Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timer = &fakeTimer{ch: make(chan time.Time)}
	return c.timer
}

func (c *fakeClock) fire() {
	c.mu.Lock()
	tm, n := c.timer, c.now
	c.mu.Unlock()
	tm.fire(n)
}

// ---- test harness ----

type harness struct {
	t       *testing.T
	tbl     *Table
	clock   *fakeClock
	ledger  *economy.Ledger
	history history.Store
	chans   map[string]chan protocol.Envelope
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	ledger := economy.NewLedger(economy.NewMemoryStore(), clock.Now)
	hist := history.NewMemStore()
	cfg := Config{ID: "t1", MaxSeats: 6, SmallBlind: 10, BigBlind: 20}
	tbl := New(cfg, Deps{
		Ledger:      ledger,
		History:     hist,
		Now:         clock.Now,
		TurnTimeout: 20 * time.Second,
		Clock:       clock,
	})
	return &harness{t: t, tbl: tbl, clock: clock, ledger: ledger, history: hist,
		chans: map[string]chan protocol.Envelope{}}
}

func (h *harness) chanFor(playerID string) chan protocol.Envelope {
	if ch, ok := h.chans[playerID]; ok {
		return ch
	}
	ch := make(chan protocol.Envelope, 256)
	h.chans[playerID] = ch
	return ch
}

func env(typ string, data any) protocol.Envelope {
	return protocol.Envelope{V: protocol.ProtocolVersion, Type: typ, Data: mustJSON(data)}
}

func (h *harness) sit(playerID string, seat int, buyIn int64) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch,
		Msg: env(protocol.CmdSitDown, cmdSitDown{TableID: "t1", Seat: seat, BuyIn: buyIn})})
}

func (h *harness) bet(playerID, kind string, amount int64) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch,
		Msg: env(protocol.CmdPlaceBet, protocol.PlaceBet{TableID: "t1", Kind: kind, Amount: amount})})
}

func (h *harness) leave(playerID string) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch,
		Msg: env(protocol.CmdLeave, struct {
			TableID string `json:"tableId"`
		}{"t1"})})
}

func (h *harness) snapshot(playerID string) tableSnapshot {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch,
		Msg: env(protocol.CmdResync, struct {
			TableID string `json:"tableId"`
		}{"t1"})})
	ev := h.waitFor(playerID, protocol.EvSnapshot)
	var snap tableSnapshot
	if err := json.Unmarshal(ev.Data, &snap); err != nil {
		h.t.Fatalf("decode snapshot: %v", err)
	}
	return snap
}

// waitFor reads playerID's channel until an event of typ arrives (2s cap).
func (h *harness) waitFor(playerID, typ string) protocol.Envelope {
	h.t.Helper()
	ch := h.chanFor(playerID)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == typ {
				return ev
			}
		case <-deadline:
			h.t.Fatalf("timed out waiting for %q on %s", typ, playerID)
		}
	}
}

// expectNone asserts no event of typ arrives within a short window.
func (h *harness) expectNone(playerID, typ string, d time.Duration) {
	h.t.Helper()
	ch := h.chanFor(playerID)
	deadline := time.After(d)
	for {
		select {
		case ev := <-ch:
			if ev.Type == typ {
				h.t.Fatalf("unexpected %q event on %s", typ, playerID)
			}
		case <-deadline:
			return
		}
	}
}

// ---- tests ----

func TestTwoPlayersHandAutoStartsWithPrivateHoleCards(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)

	da := decodeHandDealt(t, h.waitFor("alice", protocol.EvHandDealt))
	db := decodeHandDealt(t, h.waitFor("bob", protocol.EvHandDealt))

	if len(da.YourHole) != 2 || len(db.YourHole) != 2 {
		t.Fatalf("each player must get exactly 2 hole cards: %v %v", da.YourHole, db.YourHole)
	}
	if da.YourSeat != 0 || db.YourSeat != 1 {
		t.Fatalf("wrong seats in deal: alice=%d bob=%d", da.YourSeat, db.YourSeat)
	}
	if da.Commitment == "" || da.Commitment != db.Commitment {
		t.Fatalf("both deals must share the same fair commitment")
	}
	if da.YourHole[0] == db.YourHole[0] && da.YourHole[1] == db.YourHole[1] {
		t.Fatalf("players must not receive identical hole cards")
	}
}

func TestScriptedHandToShowdownAwardsAndHistory(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	seatToPlayer := map[int]string{0: "alice", 1: "bob"}
	sd := h.driveCallDownToShowdown(seatToPlayer, "alice")

	var total int64
	for _, a := range sd.Awards {
		total += int64(a.Amount)
	}
	if total != 40 { // SB 10 + BB 20, both complete to 20 -> pot 40
		t.Fatalf("awards total = %d, want pot 40", total)
	}
	if len(sd.Board) != 5 {
		t.Fatalf("showdown board should have 5 cards, got %d", len(sd.Board))
	}
	if len(sd.Revealed) != 2 {
		t.Fatalf("both non-folded players' holes must be revealed, got %d", len(sd.Revealed))
	}

	// fair reveal follows the showdown.
	fr := h.waitFor("alice", protocol.EvFairReveal)
	var reveal protocol.FairReveal
	_ = json.Unmarshal(fr.Data, &reveal)
	if reveal.Seed == "" || reveal.Commitment == "" {
		t.Fatalf("fair reveal must carry seed and commitment")
	}

	recs := h.history.ByPlayer("alice", 0)
	if len(recs) == 0 {
		t.Fatalf("hand history was not saved")
	}
	if len(recs[0].Awards) == 0 || recs[0].SeedHex == "" {
		t.Fatalf("saved record missing awards or seed reveal")
	}
}

func TestIllegalActionErrorsOnlyToActor(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	// Heads-up: seat 0 (alice, button/SB) acts first. Bob acting out of turn is illegal.
	h.bet("bob", "check", 0)
	ev := h.waitFor("bob", protocol.EvError)
	var ee protocol.ErrorEvent
	_ = json.Unmarshal(ev.Data, &ee)
	if ee.Code != "illegal_action" {
		t.Fatalf("error code = %q, want illegal_action", ee.Code)
	}
	h.expectNone("alice", protocol.EvError, 100*time.Millisecond)
}

func TestTurnTimeoutAutoFoldsAndSitsOut(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	// Alice (seat 0, SB) is to act with a live bet to call -> timeout auto-folds.
	h.clock.fire()

	bp := decodeBetPlaced(t, h.waitFor("bob", protocol.EvBetPlaced))
	if bp.Seat != 0 || bp.Kind != "fold" {
		t.Fatalf("expected seat 0 auto-fold, got seat=%d kind=%s", bp.Seat, bp.Kind)
	}

	snap := h.snapshot("bob")
	var found bool
	for _, s := range snap.Seats {
		if s.Seat == 0 {
			found = true
			if !s.SittingOut {
				t.Fatalf("timed-out seat 0 must be marked sitting out")
			}
		}
	}
	if !found {
		t.Fatalf("seat 0 missing from snapshot")
	}
}

func TestResyncReturnsCoherentSnapshot(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	snap := h.snapshot("alice")
	if !snap.HandRunning {
		t.Fatalf("snapshot should report a running hand")
	}
	if len(snap.Seats) != 2 {
		t.Fatalf("snapshot should list 2 seats, got %d", len(snap.Seats))
	}
	if snap.ToAct != 0 {
		t.Fatalf("seat 0 should be to act preflop heads-up, got %d", snap.ToAct)
	}
	if snap.Pot != 30 { // SB 10 + BB 20 posted
		t.Fatalf("pot after blinds = %d, want 30", snap.Pot)
	}
}

func TestLeaveMidHandFoldsAndCashesOut(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	// Bob (seat 1, BB, not to act) leaves mid-hand: folds, alice wins uncontested.
	startBal := h.ledger.Balance("bob") // 10000 - 1000 buy-in = 9000
	h.leave("bob")

	// Alice sees the seat update reflecting bob's departure.
	su := decodeSeatUpdate(t, h.waitFor("alice", protocol.EvSeatUpdate))
	for _, s := range su.Seats {
		if s.PlayerID == "bob" {
			t.Fatalf("bob should be unseated after leaving")
		}
	}

	// Bob posted the big blind (20); his remaining 980 is cashed out.
	if bal := h.ledger.Balance("bob"); bal != startBal+980 {
		t.Fatalf("bob balance = %d, want %d (cashed out 980)", bal, startBal+980)
	}
}

// ---- helpers ----

// driveCallDownToShowdown makes whoever is to act call until the hand settles,
// returning the decoded showdown event.
func (h *harness) driveCallDownToShowdown(seatToPlayer map[int]string, observer string) showdown {
	h.t.Helper()
	toAct := h.snapshot(observer).ToAct
	ch := h.chanFor(observer)
	for step := 0; step < 60; step++ {
		h.bet(seatToPlayer[toAct], "call", 0)
		for {
			select {
			case ev := <-ch:
				switch ev.Type {
				case protocol.EvShowdown:
					var sd showdown
					if err := json.Unmarshal(ev.Data, &sd); err != nil {
						h.t.Fatalf("decode showdown: %v", err)
					}
					return sd
				case protocol.EvBetPlaced:
					bp := decodeBetPlaced(h.t, ev)
					if bp.ToAct >= 0 {
						toAct = bp.ToAct
						goto next
					}
				}
			case <-time.After(2 * time.Second):
				h.t.Fatalf("timed out driving hand at step %d", step)
			}
		}
	next:
	}
	h.t.Fatalf("hand did not reach showdown")
	return showdown{}
}

func decodeHandDealt(t *testing.T, ev protocol.Envelope) protocol.HandDealt {
	t.Helper()
	var d protocol.HandDealt
	if err := json.Unmarshal(ev.Data, &d); err != nil {
		t.Fatalf("decode hand_dealt: %v", err)
	}
	return d
}

func decodeBetPlaced(t *testing.T, ev protocol.Envelope) betPlaced {
	t.Helper()
	var bp betPlaced
	if err := json.Unmarshal(ev.Data, &bp); err != nil {
		t.Fatalf("decode bet_placed: %v", err)
	}
	return bp
}

func decodeSeatUpdate(t *testing.T, ev protocol.Envelope) seatUpdate {
	t.Helper()
	var su seatUpdate
	if err := json.Unmarshal(ev.Data, &su); err != nil {
		t.Fatalf("decode seat_update: %v", err)
	}
	return su
}

var _ = engine.Fold // keep engine import if unused paths change
