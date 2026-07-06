package localcore

import (
	"encoding/json"
	"testing"

	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// driveScripted runs the full join/sit/start/action script and returns the
// concatenated raw event stream (in Seq order) plus the final StateHash.
func driveScripted(t *testing.T, acts []action) ([]string, string) {
	t.Helper()
	seats := []int{0, 1, 2}
	names := pids(seats)
	host := names[0]
	lt := NewLocalTable(Config{
		ID: "T1", MaxSeats: 9, SmallBlind: 1, BigBlind: 2,
		HostPlayerID: host, Private: true,
	}, seedHex)

	var stream []string
	record := func(res map[string][]json.RawMessage) {
		decodeEvents(t, res, func(recipient string, e protocol.Envelope) {
			b, _ := json.Marshal(struct {
				To string            `json:"to"`
				E  protocol.Envelope `json:"e"`
			}{recipient, e})
			stream = append(stream, string(b))
		})
	}

	for _, s := range seats {
		mustSubmit(t, lt, names[s], env(t, protocol.CmdJoinTable, nil), record)
		mustSubmit(t, lt, names[s], env(t, protocol.CmdSitDown, cmdSitDown{TableID: "T1", Seat: s, BuyIn: 100}), record)
	}
	mustSubmit(t, lt, host, env(t, protocol.CmdStartHand, nil), record)
	for _, a := range acts {
		res, err := lt.Submit(names[a.seat], env(t, protocol.CmdPlaceBet, protocol.PlaceBet{
			TableID: "T1", Kind: a.kind, Amount: a.amount,
		}))
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		record(res)
	}
	return stream, lt.StateHash()
}

// TestDeterministic asserts that identical inputs produce identical event
// streams and an identical StateHash across two independent runs.
func TestDeterministic(t *testing.T) {
	s1, h1 := driveScripted(t, script3)
	s2, h2 := driveScripted(t, script3)

	if h1 != h2 {
		t.Fatalf("StateHash differs across identical runs: %s vs %s", h1, h2)
	}
	if len(s1) != len(s2) {
		t.Fatalf("event stream length differs: %d vs %d", len(s1), len(s2))
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			t.Fatalf("event %d differs:\n a=%s\n b=%s", i, s1[i], s2[i])
		}
	}
}

// TestStateHashChangesWithState asserts the hash is sensitive to state: a hand
// in progress hashes differently from the same table between hands.
func TestStateHashChangesWithState(t *testing.T) {
	seats := []int{0, 1, 2}
	names := pids(seats)
	lt := NewLocalTable(Config{
		ID: "T1", MaxSeats: 9, SmallBlind: 1, BigBlind: 2,
		HostPlayerID: names[0], Private: true,
	}, seedHex)
	noop := func(map[string][]json.RawMessage) {}
	for _, s := range seats {
		mustSubmit(t, lt, names[s], env(t, protocol.CmdJoinTable, nil), noop)
		mustSubmit(t, lt, names[s], env(t, protocol.CmdSitDown, cmdSitDown{TableID: "T1", Seat: s, BuyIn: 100}), noop)
	}
	between := lt.StateHash()
	mustSubmit(t, lt, names[0], env(t, protocol.CmdStartHand, nil), noop)
	dealt := lt.StateHash()
	if between == dealt {
		t.Fatal("StateHash did not change when a hand was dealt")
	}
}

// TestVoidHandReturnsChips asserts VoidHand restores every seat's stack to its
// pre-hand value and returns the table to the between-hands state.
func TestVoidHandReturnsChips(t *testing.T) {
	seats := []int{0, 1, 2}
	names := pids(seats)
	lt := NewLocalTable(Config{
		ID: "T1", MaxSeats: 9, SmallBlind: 1, BigBlind: 2,
		HostPlayerID: names[0], Private: true,
	}, seedHex)
	noop := func(map[string][]json.RawMessage) {}
	for _, s := range seats {
		mustSubmit(t, lt, names[s], env(t, protocol.CmdJoinTable, nil), noop)
		mustSubmit(t, lt, names[s], env(t, protocol.CmdSitDown, cmdSitDown{TableID: "T1", Seat: s, BuyIn: 100}), noop)
	}
	mustSubmit(t, lt, names[0], env(t, protocol.CmdStartHand, nil), noop)
	// Put chips in the pot: seat 0 raises.
	if _, err := lt.Submit(names[0], env(t, protocol.CmdPlaceBet, protocol.PlaceBet{TableID: "T1", Kind: "raise", Amount: 10})); err != nil {
		t.Fatalf("raise: %v", err)
	}
	if lt.hand == nil {
		t.Fatal("expected a hand in progress")
	}
	lt.VoidHand()
	if lt.hand != nil {
		t.Fatal("VoidHand did not clear the hand")
	}
	for _, s := range seats {
		seat := s
		if lt.seats[seat].stack != 100 {
			t.Fatalf("seat %d stack = %d, want 100 after void", seat, lt.seats[seat].stack)
		}
	}
}
