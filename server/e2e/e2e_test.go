package e2e_test

import (
	"strings"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/fair"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// ---- wire payload shapes decoded on the client side (mirror table/events.go) ----

type snapshotView struct {
	TableID     string     `json:"tableId"`
	Seats       []seatView `json:"seats"`
	Button      int        `json:"button"`
	HandRunning bool       `json:"handRunning"`
	HandID      string     `json:"handId"`
	Street      string     `json:"street"`
	Board       []string   `json:"board"`
	Pot         int64      `json:"pot"`
	ToAct       int        `json:"toAct"`
}

type seatView struct {
	Seat     int    `json:"seat"`
	PlayerID string `json:"playerId"`
	Stack    int64  `json:"stack"`
}

type betPlacedView struct {
	Seat   int    `json:"seat"`
	Kind   string `json:"kind"`
	Amount int64  `json:"amount"`
	Pot    int64  `json:"pot"`
	ToAct  int    `json:"toAct"`
}

type streetAdvancedView struct {
	Street string   `json:"street"`
	Board  []string `json:"board"`
}

type awardView struct {
	SeatID int   `json:"SeatID"`
	Amount int64 `json:"Amount"`
}

type showdownView struct {
	HandID string      `json:"handId"`
	Board  []string    `json:"board"`
	Awards []awardView `json:"awards"`
}

// TestScriptedThreeClientHand boots the full stack and drives three WS clients
// through auth, private-room creation, seating, a scripted three-handed hand to
// showdown, provably-fair verification, reconnect/resync, and an illegal action.
func TestScriptedThreeClientHand(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	go func() { defer close(done); runScript(t) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("test exceeded 30s budget")
	}
}

func runScript(t *testing.T) {
	h := newHarness(t)

	// 1. Three guest identities.
	tokA, _ := h.guest(t)
	tokB, _ := h.guest(t)
	tokC, _ := h.guest(t)

	// 2. A creates a private room; B and C resolve it by code.
	tableID, code := h.createRoom(t, tokA, 10, 20, 6)
	if got := h.joinRoom(t, tokB, code); got != tableID {
		t.Fatalf("joinRoom B: tableId %q != %q", got, tableID)
	}
	if got := h.joinRoom(t, tokC, code); got != tableID {
		t.Fatalf("joinRoom C: tableId %q != %q", got, tableID)
	}

	// 3. All three connect WS, join_table, receive table_snapshot.
	a := dialClient(t, h.wsURL, tokA, "A")
	b := dialClient(t, h.wsURL, tokB, "B")
	c := dialClient(t, h.wsURL, tokC, "C")
	defer a.close()
	defer b.close()
	defer c.close()

	for _, cl := range []*wsClient{a, b, c} {
		cl.cmd(protocol.CmdJoinTable, map[string]any{"tableId": tableID})
		snap := decodeData[snapshotView](t, cl.waitFor(protocol.EvSnapshot))
		if snap.TableID != tableID {
			t.Fatalf("%s: snapshot tableId %q != %q", cl.name, snap.TableID, tableID)
		}
	}

	// 4. Seat A then B: this auto-starts an initial heads-up hand (the engine
	// deals as soon as two seats are ready). C then sits during that hand and is
	// dealt in only on the next hand. See the report note on this finding.
	seatClient := map[int]*wsClient{0: a, 1: b, 2: c}
	a.cmd(protocol.CmdSitDown, sitPayload(tableID, 0))
	b.cmd(protocol.CmdSitDown, sitPayload(tableID, 1))
	// Heads-up hand (h1) starts: A and B each get hand_dealt.
	assertHandDealt(t, a, tableID, "-h1", 0)
	assertHandDealt(t, b, tableID, "-h1", 1)

	// C sits; wait until A observes a 3-seat table so C is guaranteed seated on
	// the table loop before we end h1 (so the next hand is three-handed).
	c.cmd(protocol.CmdSitDown, sitPayload(tableID, 2))
	waitForSeatCount(t, a, 3)

	// End h1 by having its actor fold -> uncontested showdown -> settle -> the
	// three-handed hand (h2) auto-starts and deals to A, B, and C.
	foldCurrentActor(t, a, tableID, seatClient)

	// 5. h2 starts: every client receives hand_dealt with exactly 2 hole cards
	// and a 64-hex commitment.
	dealt := map[*wsClient]protocol.HandDealt{}
	for _, cl := range []*wsClient{a, b, c} {
		hd := decodeData[protocol.HandDealt](t, cl.waitFor(protocol.EvHandDealt))
		if !strings.HasSuffix(hd.HandID, "-h2") {
			t.Fatalf("%s: expected three-handed hand h2, got handId %q", cl.name, hd.HandID)
		}
		if len(hd.YourHole) != 2 {
			t.Fatalf("%s: hand_dealt has %d hole cards, want 2", cl.name, len(hd.YourHole))
		}
		if len(hd.Commitment) != 64 || !isHex(hd.Commitment) {
			t.Fatalf("%s: commitment %q is not 64 hex chars", cl.name, hd.Commitment)
		}
		dealt[cl] = hd
	}
	commitment := dealt[a].Commitment
	if dealt[b].Commitment != commitment || dealt[c].Commitment != commitment {
		t.Fatalf("clients disagree on hand commitment")
	}

	// Privacy: A must never see B's or C's hole cards pre-showdown. Mark the scan
	// start now, right after h2's hand_dealt, so h1's showdown is excluded.
	aStart := a.rawLen()
	forbidden := append(append([]string{}, dealt[b].YourHole...), dealt[c].YourHole...)

	// 9. Illegal out-of-turn action: resolve who is to act, then have a different
	// client bet. Only that client receives an error.
	snap := resync(t, a, tableID)
	toAct := snap.ToAct
	illegalSeat := 0
	if toAct == 0 {
		illegalSeat = 1
	}
	illegal := seatClient[illegalSeat]
	illegal.cmd(protocol.CmdPlaceBet, betPayload(tableID, "check", 0))
	errEv := decodeData[protocol.ErrorEvent](t, illegal.waitFor(protocol.EvError))
	if errEv.Code == "" {
		t.Fatalf("%s: illegal action error missing code", illegal.name)
	}
	for _, cl := range []*wsClient{a, b, c} {
		if cl != illegal {
			cl.expectNone(protocol.EvError, 250*time.Millisecond)
		}
	}

	// 6 & 7. Drive the scripted hand from A's event stream. First actor raises to
	// 60; everyone else calls (a call with nothing to match is a legal check on
	// this engine). Assert seq monotonicity, street progression, and showdown.
	streets, finalPot, showEv := playHandFromStream(t, a, tableID, toAct, seatClient)

	assertStreets(t, streets)

	show := decodeData[showdownView](t, showEv)
	var awarded int64
	for _, aw := range show.Awards {
		awarded += aw.Amount
	}
	if awarded != finalPot {
		t.Fatalf("awards sum %d != final pot %d", awarded, finalPot)
	}
	if len(show.Board) != 5 {
		t.Fatalf("showdown board has %d cards, want 5", len(show.Board))
	}

	// Privacy scan over A's pre-showdown traffic for this hand.
	assertNoLeak(t, a, aStart, forbidden)

	// 7 (cont). fair_reveal: seed hashes to the commitment, and the shuffle it
	// implies is consistent with the revealed board (board = deck[6:11] for a
	// three-handed deal: 6 hole cards round-robin, then flop/turn/river, no burns).
	reveal := decodeData[protocol.FairReveal](t, a.waitFor(protocol.EvFairReveal))
	if reveal.Commitment != commitment {
		t.Fatalf("fair_reveal commitment %q != hand commitment %q", reveal.Commitment, commitment)
	}
	seed, err := fair.SeedFromHex(reveal.Seed)
	if err != nil {
		t.Fatalf("SeedFromHex: %v", err)
	}
	if !fair.Verify(commitment, seed) {
		t.Fatalf("revealed seed does not hash to commitment")
	}
	deck := fair.Shuffle(seed)
	for i, boardCard := range show.Board {
		if got := deck[6+i].String(); got != boardCard {
			t.Fatalf("board[%d]=%q but recomputed deck[%d]=%q", i, boardCard, 6+i, got)
		}
	}

	// Drain h2 terminal events on B and C so their streams are clean.
	for _, cl := range []*wsClient{b, c} {
		cl.waitFor(protocol.EvShowdown)
		cl.waitFor(protocol.EvFairReveal)
	}

	// 8. Reconnect/resync: C drops its WS, redials, re-joins, resyncs, and gets a
	// coherent snapshot (correct tableId, 3 seats).
	c.close()
	c2 := dialClient(t, h.wsURL, tokC, "C2")
	defer c2.close()
	c2.cmd(protocol.CmdJoinTable, map[string]any{"tableId": tableID})
	c2.waitFor(protocol.EvSnapshot)
	c2.cmd(protocol.CmdResync, map[string]any{"tableId": tableID})
	rs := decodeData[snapshotView](t, c2.waitFor(protocol.EvSnapshot))
	if rs.TableID != tableID {
		t.Fatalf("resync snapshot tableId %q != %q", rs.TableID, tableID)
	}
	if len(rs.Seats) != 3 {
		t.Fatalf("resync snapshot has %d seats, want 3", len(rs.Seats))
	}
}

// playHandFromStream drives every action off the server's own toAct hints and
// returns the street_advanced events, the final pot, and the showdown envelope.
func playHandFromStream(t *testing.T, a *wsClient, tableID string, firstActor int, seatClient map[int]*wsClient) ([]streetAdvancedView, int64, protocol.Envelope) {
	t.Helper()
	// First action: the current actor raises to 60.
	seatClient[firstActor].cmd(protocol.CmdPlaceBet, betPayload(tableID, "raise", 60))

	var streets []streetAdvancedView
	var lastSeq uint64
	var finalPot int64
	for {
		env := a.next()
		switch env.Type {
		case protocol.EvBetPlaced:
			if env.Seq <= lastSeq {
				t.Fatalf("bet_placed seq %d not > previous %d", env.Seq, lastSeq)
			}
			lastSeq = env.Seq
			bp := decodeData[betPlacedView](t, env)
			finalPot = bp.Pot
			if bp.ToAct >= 0 {
				seatClient[bp.ToAct].cmd(protocol.CmdPlaceBet, betPayload(tableID, "call", 0))
			}
		case protocol.EvStreet:
			streets = append(streets, decodeData[streetAdvancedView](t, env))
		case protocol.EvShowdown:
			return streets, finalPot, env
		}
	}
}

func assertStreets(t *testing.T, streets []streetAdvancedView) {
	t.Helper()
	want := []struct {
		name string
		n    int
	}{{"flop", 3}, {"turn", 4}, {"river", 5}}
	if len(streets) != len(want) {
		t.Fatalf("got %d street_advanced events, want %d: %+v", len(streets), len(want), streets)
	}
	for i, w := range want {
		if streets[i].Street != w.name {
			t.Fatalf("street[%d] = %q, want %q", i, streets[i].Street, w.name)
		}
		if len(streets[i].Board) != w.n {
			t.Fatalf("%s board has %d cards, want %d", w.name, len(streets[i].Board), w.n)
		}
	}
}

// assertNoLeak scans A's pre-showdown raw traffic for any forbidden hole-card
// token. Searching for the quoted form (e.g. `"Kd"`) is collision-free: cards
// only appear as JSON array elements, never inside hex commitment/seed/tableId
// values (which contain no quotes), and every card in a deck is unique.
func assertNoLeak(t *testing.T, a *wsClient, start int, forbidden []string) {
	t.Helper()
	msgs := a.rawWindowUntilShowdown(start)
	var sb strings.Builder
	for _, m := range msgs {
		sb.Write(m)
		sb.WriteByte('\n')
	}
	hay := sb.String()
	for _, card := range forbidden {
		if strings.Contains(hay, `"`+card+`"`) {
			t.Fatalf("privacy leak: opponent hole card %q found in A's pre-showdown traffic", card)
		}
	}
}

func assertHandDealt(t *testing.T, cl *wsClient, tableID, handSuffix string, wantSeat int) {
	t.Helper()
	hd := decodeData[protocol.HandDealt](t, cl.waitFor(protocol.EvHandDealt))
	if !strings.HasSuffix(hd.HandID, handSuffix) {
		t.Fatalf("%s: handId %q lacks suffix %q", cl.name, hd.HandID, handSuffix)
	}
	if hd.YourSeat != wantSeat {
		t.Fatalf("%s: yourSeat %d, want %d", cl.name, hd.YourSeat, wantSeat)
	}
}

func waitForSeatCount(t *testing.T, cl *wsClient, n int) {
	t.Helper()
	deadline := time.After(waitTimeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("%s: timed out waiting for %d seats", cl.name, n)
		default:
		}
		env := cl.waitFor(protocol.EvSeatUpdate)
		su := decodeData[struct {
			Seats []seatView `json:"seats"`
		}](t, env)
		if len(su.Seats) == n {
			return
		}
	}
}

func foldCurrentActor(t *testing.T, a *wsClient, tableID string, seatClient map[int]*wsClient) {
	t.Helper()
	snap := resync(t, a, tableID)
	if snap.ToAct < 0 {
		t.Fatalf("no actor to fold; snapshot: %+v", snap)
	}
	seatClient[snap.ToAct].cmd(protocol.CmdPlaceBet, betPayload(tableID, "fold", 0))
}

func resync(t *testing.T, cl *wsClient, tableID string) snapshotView {
	t.Helper()
	cl.cmd(protocol.CmdResync, map[string]any{"tableId": tableID})
	return decodeData[snapshotView](t, cl.waitFor(protocol.EvSnapshot))
}

func sitPayload(tableID string, seat int) map[string]any {
	return map[string]any{"tableId": tableID, "seat": seat, "buyIn": 2000}
}

func betPayload(tableID, kind string, amount int64) map[string]any {
	return map[string]any{"tableId": tableID, "kind": kind, "amount": amount}
}

func isHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
