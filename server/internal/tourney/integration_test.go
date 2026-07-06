package tourney

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// --- deterministic clock with a never-firing timer ---
//
// Turn deadlines are irrelevant to these tests (the driver always acts), so the
// timer never fires; blind levels advance purely by moving Now forward, which is
// how the real clock drives the schedule too.

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func (c *fakeClock) NewTimer() table.Timer { return noopTimer{} }

type noopTimer struct{}

func (noopTimer) C() <-chan time.Time  { return nil } // never fires
func (noopTimer) Reset(time.Duration)  {}
func (noopTimer) Stop()                {}

// clockFactory builds real tournament tables wired to the shared fake clock and
// ledger plus the SNG controller, so the whole path (registration -> auto-start
// -> real table -> controller -> payouts) is exercised deterministically.
type clockFactory struct {
	clock  *fakeClock
	ledger *economy.Ledger
	tables map[string]*table.Table
}

func (f *clockFactory) CreateTourney(cfg table.Config, onComplete table.OnHandComplete) *table.Table {
	tbl := table.New(cfg, table.Deps{
		Ledger:         f.ledger,
		Now:            f.clock.Now,
		Clock:          f.clock,
		OnHandComplete: onComplete,
	})
	f.tables[cfg.ID] = tbl
	return tbl
}

func TestFullSNGRegistersAutoStartsAndDealsFirstHand(t *testing.T) {
	clk := &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	ledger := economy.NewLedger(economy.NewMemoryStore(), clk.Now)
	factory := &clockFactory{clock: clk, ledger: ledger, tables: map[string]*table.Table{}}
	m := NewManager(ledger, factory)
	m.now = clk.Now

	sngID, tableID, err := m.Create(DefaultConfig("Deterministic SNG", 3, 500))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	players := []string{"p0", "p1", "p2"}
	for _, p := range players {
		if err := m.Register(sngID, p); err != nil {
			t.Fatalf("register %s: %v", p, err)
		}
	}

	// Each player was debited the buy-in; the prize pool is the sum.
	for _, p := range players {
		if bal := ledger.Balance(p); bal != economy.StartingBalance-500 {
			t.Fatalf("%s balance = %d, want %d after buy-in", p, bal, economy.StartingBalance-500)
		}
	}

	tbl, ok := factory.tables[tableID]
	if !ok {
		t.Fatalf("auto-start did not create the reserved table %q", tableID)
	}

	// Players connect via the table command API; the first hand deals only once
	// all three are subscribed (tournament gate).
	chans := map[string]chan protocol.Envelope{}
	for i, p := range players {
		ch := make(chan protocol.Envelope, 256)
		chans[p] = ch
		tbl.Submit(table.Command{PlayerID: p, Reply: ch, Msg: joinMsg(tableID)})
		if i < len(players)-1 {
			// Not everyone connected yet: no deal.
			expectNo(t, chans[players[0]], protocol.EvHandDealt, 30*time.Millisecond)
		}
	}

	// All connected: every player is dealt into hand 1 at level-1 blinds with the
	// tournament starting stack.
	for _, p := range players {
		hd := waitForEvent(t, chans[p], protocol.EvHandDealt)
		var d protocol.HandDealt
		if err := json.Unmarshal(hd.Data, &d); err != nil {
			t.Fatalf("decode hand_dealt: %v", err)
		}
		if d.Blinds != [2]int64{10, 20} {
			t.Fatalf("%s dealt at blinds %v, want level-1 10/20", p, d.Blinds)
		}
		if len(d.YourHole) != 2 {
			t.Fatalf("%s must get 2 hole cards, got %v", p, d.YourHole)
		}
	}

	// Snapshot confirms three seats each at the 1500 starting stack (chips on the
	// table = stack + any posted blind).
	snap := resync(t, tbl, "p0", chans["p0"])
	if len(snap.Seats) != 3 {
		t.Fatalf("expected 3 tournament seats, got %d", len(snap.Seats))
	}
	for _, s := range snap.Seats {
		if s.Stack+s.Committed != 1500 {
			t.Fatalf("seat %d chips = %d, want 1500 starting stack", s.Seat, s.Stack+s.Committed)
		}
	}
}

// --- small event helpers (this test package cannot see table's unexported ones) ---

func joinMsg(tableID string) protocol.Envelope {
	return protocol.Envelope{V: protocol.ProtocolVersion, Type: protocol.CmdJoinTable,
		Data: mustJSON(struct {
			TableID string `json:"tableId"`
		}{tableID})}
}

func resyncMsg(tableID string) protocol.Envelope {
	return protocol.Envelope{V: protocol.ProtocolVersion, Type: protocol.CmdResync,
		Data: mustJSON(struct {
			TableID string `json:"tableId"`
		}{tableID})}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func waitForEvent(t *testing.T, ch chan protocol.Envelope, typ string) protocol.Envelope {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type == typ {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %q", typ)
		}
	}
}

func expectNo(t *testing.T, ch chan protocol.Envelope, typ string, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case ev := <-ch:
			if ev.Type == typ {
				t.Fatalf("unexpected %q event", typ)
			}
		case <-deadline:
			return
		}
	}
}

// snapshotShape mirrors the fields of table's unexported snapshot we assert on.
type snapshotShape struct {
	Seats []struct {
		Seat      int   `json:"seat"`
		Stack     int64 `json:"stack"`
		Committed int64 `json:"committed"`
	} `json:"seats"`
}

func resync(t *testing.T, tbl *table.Table, playerID string, ch chan protocol.Envelope) snapshotShape {
	t.Helper()
	tbl.Submit(table.Command{PlayerID: playerID, Reply: ch, Msg: resyncMsg(tbl.Cfg.ID)})
	ev := waitForEvent(t, ch, protocol.EvSnapshot)
	var snap snapshotShape
	if err := json.Unmarshal(ev.Data, &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	return snap
}
