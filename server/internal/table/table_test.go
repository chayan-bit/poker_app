package table

import (
	"encoding/json"
	"strings"
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
func (f *fakeTimer) Reset(time.Duration) { f.mu.Lock(); f.armed = true; f.mu.Unlock() }
func (f *fakeTimer) Stop()               { f.mu.Lock(); f.armed = false; f.mu.Unlock() }

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

// advance moves the fake clock forward so Now-gated deadlines become due.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// advanceAndFire advances the clock then fires the loop timer, the standard way
// to trip a deadline deterministically.
func (c *fakeClock) advanceAndFire(d time.Duration) {
	c.advance(d)
	c.fire()
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
	return newHarnessCfg(t, Config{ID: "t1", MaxSeats: 6, SmallBlind: 10, BigBlind: 20}, Deps{})
}

// newHarnessCfg builds a table with an explicit config and partial deps (fake
// clock and in-memory ledger/history are always injected; any zero timeout in
// deps is left to withDefaults).
func newHarnessCfg(t *testing.T, cfg Config, deps Deps) *harness {
	t.Helper()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	ledger := economy.NewLedger(economy.NewMemoryStore(), clock.Now)
	hist := history.NewMemStore()
	deps.Ledger = ledger
	deps.History = hist
	deps.Now = clock.Now
	deps.Clock = clock
	if deps.TurnTimeout == 0 {
		deps.TurnTimeout = 20 * time.Second
	}
	tbl := New(cfg, deps)
	return &harness{t: t, tbl: tbl, clock: clock, ledger: ledger, history: hist,
		chans: map[string]chan protocol.Envelope{}}
}

// newLoopless builds a Table WITHOUT starting its loop goroutine, so in-package
// tests can call table methods synchronously (no data race). Used to exercise
// evictBrokePlayers deterministically without controlling the shuffle.
func newLoopless(t *testing.T, cfg Config) *Table {
	t.Helper()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	d := Deps{
		Ledger: economy.NewLedger(economy.NewMemoryStore(), clock.Now),
		Clock:  clock,
	}.withDefaults()
	return &Table{
		Cfg: cfg, deps: d,
		seats: map[int]*seatState{}, subs: map[string]chan<- protocol.Envelope{},
		inbox: make(chan Command, 64), timer: d.Clock.NewTimer(),
		startStack: map[int]engine.Chips{}, done: make(chan struct{}),
	}
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

// tableIDMsg is the common {tableId} body for parameterless commands.
func tableIDMsg(typ string) protocol.Envelope {
	return env(typ, struct {
		TableID string `json:"tableId"`
	}{"t1"})
}

func (h *harness) join(playerID string) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch, Msg: tableIDMsg(protocol.CmdJoinTable)})
}

func (h *harness) startHand(playerID string) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch, Msg: tableIDMsg(protocol.CmdStartHand)})
}

func (h *harness) rebuy(playerID string, amount int64) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch,
		Msg: env(protocol.CmdRebuy, cmdRebuy{TableID: "t1", Amount: amount})})
}

func (h *harness) sitOut(playerID string) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch, Msg: tableIDMsg(protocol.CmdSitOut)})
}

func (h *harness) sitIn(playerID string) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch, Msg: tableIDMsg(protocol.CmdSitIn)})
}

func (h *harness) disconnect(playerID string) {
	ch := h.chanFor(playerID)
	h.tbl.Submit(Command{PlayerID: playerID, Reply: ch, Msg: tableIDMsg(protocol.CmdDisconnected)})
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
	h.clock.advanceAndFire(20 * time.Second)

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

// ---- issue #11: host-start gating ----

func TestPrivateRoomWaitsForHostThenHostStarts(t *testing.T) {
	h := newHarnessCfg(t, Config{ID: "t1", Visibility: Private, MaxSeats: 6, SmallBlind: 10, BigBlind: 20}, Deps{})
	h.sit("alice", 0, 1000) // first to sit -> host
	h.sit("bob", 1, 1000)

	// A table_status announces the room is waiting for the host.
	ts := decodeTableStatus(t, h.waitFor("alice", protocol.EvTableStatus))
	if !ts.WaitingForHost || ts.SeatedCount != 2 {
		t.Fatalf("table_status = %+v, want waitingForHost with 2 seated", ts)
	}
	// No hand auto-starts.
	h.expectNone("alice", protocol.EvHandDealt, 100*time.Millisecond)

	// A non-host cannot start the hand.
	h.startHand("bob")
	ee := decodeError(t, h.waitFor("bob", protocol.EvError))
	if ee.Code != "not_host" {
		t.Fatalf("bob start_hand error = %q, want not_host", ee.Code)
	}

	// The host starts; the hand deals.
	h.startHand("alice")
	da := decodeHandDealt(t, h.waitFor("alice", protocol.EvHandDealt))
	if len(da.YourHole) != 2 {
		t.Fatalf("host-started hand must deal 2 hole cards, got %v", da.YourHole)
	}
}

// ---- issue #15: rebuy bounds ----

func TestRebuyBoundsAndTiming(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000) // single seat, no hand running
	h.waitForSeatStack("alice", 0, 1000)

	// Valid rebuy raises the stack.
	h.rebuy("alice", 500)
	h.waitForSeatStack("alice", 0, 1500)

	// A rebuy that would exceed 1000*BigBlind is rejected.
	h.rebuy("alice", 1_000_000)
	if ee := decodeError(t, h.waitFor("alice", protocol.EvError)); ee.Code != "bad_rebuy" {
		t.Fatalf("oversized rebuy error = %q, want bad_rebuy", ee.Code)
	}

	// Start a hand (bob sits, public table auto-starts); rebuy is now rejected.
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.rebuy("alice", 100)
	if ee := decodeError(t, h.waitFor("alice", protocol.EvError)); ee.Code != "hand_in_progress" {
		t.Fatalf("mid-hand rebuy error = %q, want hand_in_progress", ee.Code)
	}
}

// ---- issue #15: sit_out semantics ----

func TestSitOutMidHandImmediateFoldWhenTheirTurn(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	// Alice (seat 0) is to act preflop; sit_out folds her immediately.
	h.sitOut("alice")
	bp := decodeBetPlaced(t, h.waitFor("bob", protocol.EvBetPlaced))
	if bp.Seat != 0 || bp.Kind != "fold" {
		t.Fatalf("sit_out on turn should fold seat 0, got seat=%d kind=%s", bp.Seat, bp.Kind)
	}
	if !seatInSnapshot(h.snapshot("bob"), 0).SittingOut {
		t.Fatalf("sat-out seat 0 must be marked sitting out")
	}
}

func TestSitOutWhenNotTheirTurnFoldsWhenReached(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	// Bob (seat 1, BB) is NOT to act; sit_out just marks him fold-pending.
	h.sitOut("bob")
	// Alice acts; action reaches bob, who auto-folds.
	h.bet("alice", "call", 0)

	// Among the resulting bet_placed events, bob (seat 1) must fold.
	if !sawFold(h, "alice", 1) {
		t.Fatalf("bob should auto-fold when action reaches his sat-out seat")
	}
}

// ---- issue #15: sit_in re-entry after a timeout-induced sit-out ----

func TestSitInReEntersAfterTimeout(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	// Alice times out -> auto-fold + sitting out. Bob wins uncontested, and the
	// next hand cannot start (only bob is eligible).
	h.clock.advanceAndFire(20 * time.Second)
	h.waitFor("bob", protocol.EvShowdown)
	if !seatInSnapshot(h.snapshot("bob"), 0).SittingOut {
		t.Fatalf("timed-out seat 0 should be sitting out")
	}

	// sit_in clears the timeout-induced flag; with two eligible seats a new hand
	// deals to alice.
	h.sitIn("alice")
	da := decodeHandDealt(t, h.waitFor("alice", protocol.EvHandDealt))
	if !strings.HasSuffix(da.HandID, "-h2") {
		t.Fatalf("expected the re-entry hand h2, got %q", da.HandID)
	}
}

// ---- issue #15: broke-player auto-unseat ----

func TestBrokePlayerUnseatAfterThreeHands(t *testing.T) {
	tbl := newLoopless(t, Config{ID: "t1", MaxSeats: 6, SmallBlind: 10, BigBlind: 20})
	ch := make(chan protocol.Envelope, 16)
	tbl.subs["b"] = ch
	tbl.seats[0] = &seatState{playerID: "a", stack: 0, sittingOut: true, brokeAtHand: 1}
	tbl.seats[1] = &seatState{playerID: "b", stack: 1000}

	// Two hands after going broke: not yet evicted.
	tbl.handNum = 3
	tbl.evictBrokePlayers()
	if _, ok := tbl.seats[0]; !ok {
		t.Fatalf("broke seat evicted too early (only 2 hands)")
	}

	// Third hand after going broke: evicted, and a seat_update is broadcast.
	tbl.handNum = 4
	tbl.evictBrokePlayers()
	if _, ok := tbl.seats[0]; ok {
		t.Fatalf("broke seat should be unseated after 3 hands")
	}
	select {
	case ev := <-ch:
		if ev.Type != protocol.EvSeatUpdate {
			t.Fatalf("expected seat_update on eviction, got %q", ev.Type)
		}
	default:
		t.Fatalf("eviction did not broadcast a seat_update")
	}
}

// ---- issue #15: idle shutdown ----

func TestIdleShutdownRemovesFromRegistryAndExits(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := &fakeClock{now: now}
	reg := NewRegistryWithDeps(Deps{Clock: clock, Now: clock.Now, IdleTimeout: 5 * time.Minute})
	tbl := reg.Create(Config{ID: "idle1"})

	// No subscribers, no seats: the idle clock is running. Trip it.
	clock.advanceAndFire(5 * time.Minute)

	select {
	case <-tbl.Done():
	case <-time.After(2 * time.Second):
		t.Fatalf("table loop did not exit on idle timeout")
	}
	if _, ok := reg.Get("idle1"); ok {
		t.Fatalf("idle table was not removed from the registry")
	}
}

// ---- issue #16: disconnect grace + reclaim ----

func TestDisconnectGraceSitsOut(t *testing.T) {
	h := newHarnessCfg(t, Config{ID: "t1", MaxSeats: 6, SmallBlind: 10, BigBlind: 20},
		Deps{TurnTimeout: 60 * time.Second, DisconnectGrace: 30 * time.Second})
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	// Bob's socket drops: seat is flagged disconnected, NOT folded or unseated.
	h.disconnect("bob")
	if !seatInSnapshotUpdate(t, h, "alice", 1).Disconnected {
		t.Fatalf("seat 1 must be marked disconnected during grace")
	}

	// Grace (30s) elapses before the turn timer (60s): the seat is sat out.
	h.clock.advanceAndFire(30 * time.Second)
	sv := seatInSnapshotUpdate(t, h, "alice", 1)
	if !sv.SittingOut || sv.Disconnected {
		t.Fatalf("after grace, seat 1 must be sitting out and no longer disconnected: %+v", sv)
	}
}

func TestReclaimRestoresOwnHoleCards(t *testing.T) {
	h := newHarnessCfg(t, Config{ID: "t1", MaxSeats: 6, SmallBlind: 10, BigBlind: 20},
		Deps{TurnTimeout: 60 * time.Second, DisconnectGrace: 30 * time.Second})
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	db := decodeHandDealt(t, h.waitFor("bob", protocol.EvHandDealt))
	h.waitFor("alice", protocol.EvHandDealt)

	h.disconnect("bob")

	// Bob reclaims by re-joining; his personalized snapshot restores his cards.
	h.join("bob")
	snap := decodeSnapshot(t, h.waitFor("bob", protocol.EvSnapshot))
	if snap.YourSeat != 1 {
		t.Fatalf("reclaim snapshot yourSeat = %d, want 1", snap.YourSeat)
	}
	if len(snap.YourHole) != 2 || snap.YourHole[0] != db.YourHole[0] || snap.YourHole[1] != db.YourHole[1] {
		t.Fatalf("reclaim must restore bob's own hole cards %v, got %v", db.YourHole, snap.YourHole)
	}
}

// ---- issue #12: toCall / currentBet on bet_placed ----

func TestBetPlacedCarriesCurrentBetAndToCall(t *testing.T) {
	h := newHarness(t)
	h.sit("alice", 0, 1000)
	h.sit("bob", 1, 1000)
	h.waitFor("alice", protocol.EvHandDealt)
	h.waitFor("bob", protocol.EvHandDealt)

	// Alice (SB, committed 10) raises to 60. Next to act is bob (committed 20).
	h.bet("alice", "raise", 60)
	bp := decodeBetPlaced(t, h.waitFor("bob", protocol.EvBetPlaced))
	if bp.CurrentBet != 60 {
		t.Fatalf("currentBet = %d, want 60", bp.CurrentBet)
	}
	if bp.ToAct != 1 {
		t.Fatalf("toAct = %d, want seat 1", bp.ToAct)
	}
	if bp.ToCall != 40 { // 60 - bob's committed 20
		t.Fatalf("toCall = %d, want 40", bp.ToCall)
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

func decodeTableStatus(t *testing.T, ev protocol.Envelope) tableStatus {
	t.Helper()
	var ts tableStatus
	if err := json.Unmarshal(ev.Data, &ts); err != nil {
		t.Fatalf("decode table_status: %v", err)
	}
	return ts
}

func decodeError(t *testing.T, ev protocol.Envelope) protocol.ErrorEvent {
	t.Helper()
	var ee protocol.ErrorEvent
	if err := json.Unmarshal(ev.Data, &ee); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	return ee
}

func decodeSnapshot(t *testing.T, ev protocol.Envelope) tableSnapshot {
	t.Helper()
	var snap tableSnapshot
	if err := json.Unmarshal(ev.Data, &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return snap
}

// seatInSnapshot returns the seatView for seat in snap, failing if absent.
func seatInSnapshot(snap tableSnapshot, seat int) seatView {
	for _, s := range snap.Seats {
		if s.Seat == seat {
			return s
		}
	}
	return seatView{Seat: -1}
}

// seatInSnapshotUpdate waits for the next seat_update on observer and returns the
// named seat's view.
func seatInSnapshotUpdate(t *testing.T, h *harness, observer string, seat int) seatView {
	t.Helper()
	su := decodeSeatUpdate(t, h.waitFor(observer, protocol.EvSeatUpdate))
	for _, s := range su.Seats {
		if s.Seat == seat {
			return s
		}
	}
	t.Fatalf("seat %d missing from seat_update", seat)
	return seatView{}
}

// waitForSeatStack waits for a seat_update showing seat at the given stack.
func (h *harness) waitForSeatStack(observer string, seat int, stack int64) {
	h.t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			h.t.Fatalf("timed out waiting for seat %d stack %d", seat, stack)
		default:
		}
		su := decodeSeatUpdate(h.t, h.waitFor(observer, protocol.EvSeatUpdate))
		for _, s := range su.Seats {
			if s.Seat == seat && s.Stack == stack {
				return
			}
		}
	}
}

// sawFold reports whether a bet_placed folding seat arrives on observer's stream.
func sawFold(h *harness, observer string, seat int) bool {
	deadline := time.After(2 * time.Second)
	ch := h.chanFor(observer)
	for {
		select {
		case ev := <-ch:
			if ev.Type != protocol.EvBetPlaced {
				continue
			}
			bp := decodeBetPlaced(h.t, ev)
			if bp.Seat == seat && bp.Kind == "fold" {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

var _ = engine.Fold // keep engine import if unused paths change
