package localcore

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/fair"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// seedHex is a fixed 32-byte seed so the deal is reproducible across the native
// engine path and the localcore facade.
const seedHex = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

// action is one scripted betting action: which seat, what kind, and the to-amount.
type action struct {
	seat   int
	kind   string
	amount int64
}

// script3 is a valid 3-handed sequence that limps preflop and checks down to
// showdown. Preflop first-to-act is seat 0 (button), then blinds; postflop the
// order is 1, 2, 0.
var script3 = []action{
	{0, "call", 0}, {1, "call", 0}, {2, "check", 0}, // preflop
	{1, "check", 0}, {2, "check", 0}, {0, "check", 0}, // flop
	{1, "check", 0}, {2, "check", 0}, {0, "check", 0}, // turn
	{1, "check", 0}, {2, "check", 0}, {0, "check", 0}, // river
}

// nativeResult is the ground truth computed directly from internal/engine +
// internal/fair, bypassing the facade entirely.
type nativeResult struct {
	hole   map[int][]string // seat -> hole cards
	board  []string
	awards []engine.Award
	stacks map[int]int64
}

func computeNative(t *testing.T, seats []int, buyIn int64, sb, bb int64, buttonPos int, acts []action) nativeResult {
	t.Helper()
	seed, err := fair.SeedFromHex(seedHex)
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	deck := fair.Shuffle(seed)
	players := make([]engine.Player, len(seats))
	for i, s := range seats {
		players[i] = engine.Player{SeatID: s, Stack: engine.Chips(buyIn), Status: engine.Active}
	}
	h, err := engine.NewHand(engine.HandConfig{
		Players: players, Deck: deck, ButtonPos: buttonPos,
		SmallBlind: engine.Chips(sb), BigBlind: engine.Chips(bb),
	})
	if err != nil {
		t.Fatalf("NewHand: %v", err)
	}
	hole := map[int][]string{}
	for _, p := range h.Players {
		hole[p.SeatID] = []string{p.Hole[0].String(), p.Hole[1].String()}
	}
	for i, a := range acts {
		kind := actionKinds[a.kind]
		nh, err := h.Apply(engine.Action{SeatID: a.seat, Kind: kind, Amount: engine.Chips(a.amount)})
		if err != nil {
			t.Fatalf("native apply step %d (%+v): %v", i, a, err)
		}
		h = nh
	}
	if h.Street != engine.Showdown {
		t.Fatalf("native hand did not reach showdown, street=%v", h.Street)
	}
	settled, err := engine.Settle(h)
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	stacks := map[int]int64{}
	for _, p := range settled.Players {
		stacks[p.SeatID] = int64(p.Stack)
	}
	return nativeResult{hole: hole, board: cardsToStrings(h.Board), awards: settled.Awards, stacks: stacks}
}

// facadeRun drives the same seed + script through the localcore facade and
// collects the observable outcomes.
type facadeRun struct {
	hole   map[int][]string
	board  []string
	awards []engine.Award
	stacks map[int]int64
}

func pids(seats []int) map[int]string {
	m := map[int]string{}
	for _, s := range seats {
		m[s] = "p" + string(rune('0'+s))
	}
	return m
}

func env(t *testing.T, typ string, data any) []byte {
	t.Helper()
	var raw json.RawMessage
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		raw = b
	}
	b, err := json.Marshal(protocol.Envelope{V: protocol.ProtocolVersion, Type: typ, Data: raw})
	if err != nil {
		t.Fatalf("marshal env: %v", err)
	}
	return b
}

// decodeEvents walks a facade result map and calls fn for every envelope,
// tagged with its recipient key, in ascending Seq order so a caller sees a
// stable stream.
func decodeEvents(t *testing.T, res map[string][]json.RawMessage, fn func(recipient string, e protocol.Envelope)) {
	t.Helper()
	type tagged struct {
		recipient string
		e         protocol.Envelope
	}
	var all []tagged
	for recipient, evs := range res {
		for _, raw := range evs {
			var e protocol.Envelope
			if err := json.Unmarshal(raw, &e); err != nil {
				t.Fatalf("decode env: %v", err)
			}
			all = append(all, tagged{recipient, e})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].e.Seq < all[j].e.Seq })
	for _, tg := range all {
		fn(tg.recipient, tg.e)
	}
}

func runFacade(t *testing.T, seats []int, buyIn, sb, bb int64, acts []action) facadeRun {
	t.Helper()
	names := pids(seats)
	host := names[seats[0]]
	lt := NewLocalTable(Config{
		ID: "T1", MaxSeats: 9, SmallBlind: sb, BigBlind: bb,
		HostPlayerID: host, Private: true,
	}, seedHex)

	fr := facadeRun{hole: map[int][]string{}, stacks: map[int]int64{}}
	collect := func(res map[string][]json.RawMessage) {
		decodeEvents(t, res, func(recipient string, e protocol.Envelope) {
			switch e.Type {
			case protocol.EvHandDealt:
				var hd protocol.HandDealt
				mustUnmarshal(t, e.Data, &hd)
				fr.hole[hd.YourSeat] = hd.YourHole
			case protocol.EvShowdown:
				var sd showdown
				mustUnmarshal(t, e.Data, &sd)
				fr.board = sd.Board
				fr.awards = sd.Awards
			case protocol.EvSeatUpdate:
				var su seatUpdate
				mustUnmarshal(t, e.Data, &su)
				for _, sv := range su.Seats {
					fr.stacks[sv.Seat] = sv.Stack
				}
			}
		})
	}

	for _, s := range seats {
		mustSubmit(t, lt, names[s], env(t, protocol.CmdJoinTable, nil), collect)
		mustSubmit(t, lt, names[s], env(t, protocol.CmdSitDown, cmdSitDown{TableID: "T1", Seat: s, BuyIn: buyIn}), collect)
	}
	// Host deals once all three are seated.
	mustSubmit(t, lt, host, env(t, protocol.CmdStartHand, nil), collect)

	for i, a := range acts {
		res, err := lt.Submit(names[a.seat], env(t, protocol.CmdPlaceBet, protocol.PlaceBet{
			TableID: "T1", Kind: a.kind, Amount: a.amount,
		}))
		if err != nil {
			t.Fatalf("facade submit step %d: %v", i, err)
		}
		collect(res)
	}
	// Final authoritative stacks live on the seats after settle writes them back;
	// no seat_update is broadcast at showdown (matching the table package), so
	// read them from the table state directly.
	for _, s := range seats {
		fr.stacks[s] = int64(lt.seats[s].stack)
	}
	return fr
}

func mustSubmit(t *testing.T, lt *LocalTable, pid string, envelope []byte, collect func(map[string][]json.RawMessage)) {
	t.Helper()
	res, err := lt.Submit(pid, envelope)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	collect(res)
}

func mustUnmarshal(t *testing.T, data json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

// TestFacadeMatchesEngine asserts the localcore facade produces the same hole
// cards, board, winners, and final stacks as internal/engine + internal/fair
// driven directly, for a fixed seed and scripted 3-player hand.
func TestFacadeMatchesEngine(t *testing.T) {
	seats := []int{0, 1, 2}
	native := computeNative(t, seats, 100, 1, 2, 0, script3)
	fr := runFacade(t, seats, 100, 1, 2, script3)

	if !reflect.DeepEqual(native.hole, fr.hole) {
		t.Fatalf("hole cards differ:\n native=%v\n facade=%v", native.hole, fr.hole)
	}
	if !reflect.DeepEqual(native.board, fr.board) {
		t.Fatalf("board differs:\n native=%v\n facade=%v", native.board, fr.board)
	}
	if !reflect.DeepEqual(sortAwards(native.awards), sortAwards(fr.awards)) {
		t.Fatalf("awards differ:\n native=%v\n facade=%v", native.awards, fr.awards)
	}
	// Native stacks are post-settle within the hand; facade seat stacks are the
	// same value written back to the seats at showdown.
	if !reflect.DeepEqual(native.stacks, fr.stacks) {
		t.Fatalf("stacks differ:\n native=%v\n facade=%v", native.stacks, fr.stacks)
	}
}

func sortAwards(a []engine.Award) []engine.Award {
	out := append([]engine.Award(nil), a...)
	sort.Slice(out, func(i, j int) bool { return out[i].SeatID < out[j].SeatID })
	return out
}
