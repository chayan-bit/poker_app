package localcore

import (
	"fmt"
	"sort"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/fair"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// eligibleSeats returns seat IDs dealt into the next hand (seated, with chips,
// not sitting out) in ascending seat order.
func (lt *LocalTable) eligibleSeats() []int {
	var out []int
	for id, s := range lt.seats {
		if s.stack > 0 && !s.sittingOut {
			out = append(out, id)
		}
	}
	sort.Ints(out)
	return out
}

// startHandIfReady deals a new hand when two or more seats are ready, none is
// running, and a fresh seed is available. It commits the fair seed before
// dealing (protocol invariant), deals per-player hole cards privately, and arms
// the turn timer. A private room waits for the host's first start_hand.
func (lt *LocalTable) startHandIfReady(o *out) {
	if lt.hand != nil {
		return
	}
	eligible := lt.eligibleSeats()
	if len(eligible) < 2 {
		return
	}
	if !lt.autoStart() && !lt.hostStarted {
		o.broadcast(protocol.EvTableStatus, tableStatus{
			TableID: lt.cfg.ID, WaitingForHost: true, SeatedCount: len(lt.seats),
		})
		return
	}
	// Never deal without a committed seed. The seed is an external input consumed
	// per hand, so a missing seed pauses dealing until setSeed supplies one.
	seed, err := fair.SeedFromHex(lt.pendingSeed)
	if err != nil {
		return
	}
	lt.pendingSeed = ""
	lt.commitment = seed.Commitment()
	lt.seedHex = seed.Hex()
	deck := fair.Shuffle(seed)

	buttonPos := lt.buttonPos(eligible)
	players := make([]engine.Player, len(eligible))
	for i, seat := range eligible {
		players[i] = engine.Player{SeatID: seat, Stack: lt.seats[seat].stack, Status: engine.Active}
	}

	hand, err := engine.NewHand(engine.HandConfig{
		Players:    players,
		Deck:       deck,
		ButtonPos:  buttonPos,
		SmallBlind: engine.Chips(lt.cfg.SmallBlind),
		BigBlind:   engine.Chips(lt.cfg.BigBlind),
	})
	if err != nil {
		return
	}

	lt.button = eligible[buttonPos]
	lt.handNum++
	for _, s := range lt.seats {
		s.foldPending = false
	}
	lt.handID = fmt.Sprintf("%s-h%d", lt.cfg.ID, lt.handNum)
	lt.hand = &hand
	lt.recordStartStacks(eligible)
	lt.dealHoleCards(o)
	lt.armTimer()
}

// buttonPos returns the index into eligible of the dealer button: the current
// button seat if still eligible, else the lowest eligible seat above it.
func (lt *LocalTable) buttonPos(eligible []int) int {
	for i, seat := range eligible {
		if seat == lt.button {
			return i
		}
	}
	for i, seat := range eligible {
		if seat > lt.button {
			return i
		}
	}
	return 0
}

// recordStartStacks snapshots each dealt seat's stack at hand start so VoidHand
// can return every committed chip if the hand is aborted mid-flight.
func (lt *LocalTable) recordStartStacks(eligible []int) {
	lt.startStack = map[int]engine.Chips{}
	for _, seat := range eligible {
		lt.startStack[seat] = lt.seats[seat].stack
	}
}

// dealHoleCards sends each connected dealt-in player only their own two cards.
func (lt *LocalTable) dealHoleCards(o *out) {
	for _, p := range lt.hand.Players {
		s, ok := lt.seats[p.SeatID]
		if !ok || !lt.connected[s.playerID] {
			continue
		}
		o.sendTo(s.playerID, protocol.EvHandDealt, protocol.HandDealt{
			TableID:    lt.cfg.ID,
			HandID:     lt.handID,
			Commitment: lt.commitment,
			YourSeat:   p.SeatID,
			YourHole:   []string{p.Hole[0].String(), p.Hole[1].String()},
			ButtonSeat: lt.button,
			Blinds:     [2]int64{lt.cfg.SmallBlind, lt.cfg.BigBlind},
		})
	}
}

// applyAction runs one action through the engine and emits the resulting events
// (bet_placed, street_advanced, showdown). It returns the engine error unchanged
// so the caller can report it to the actor only.
func (lt *LocalTable) applyAction(o *out, seat int, act engine.Action, kind string) error {
	prevStreet := lt.hand.Street

	nh, err := lt.hand.Apply(act)
	if err != nil {
		return err
	}
	lt.hand = &nh

	o.broadcast(protocol.EvBetPlaced, betPlaced{
		Seat: seat, Kind: kind, Amount: int64(act.Amount),
		Pot: int64(nh.Pot), ToAct: lt.toActSeat(),
		CurrentBet: lt.currentBet(), ToCall: lt.toCall(),
	})

	if nh.Street != prevStreet && nh.Street != engine.Showdown {
		o.broadcast(protocol.EvStreet, streetAdvanced{
			Street: streetName(nh.Street), Board: cardsToStrings(nh.Board),
		})
	}

	if nh.Street == engine.Showdown {
		lt.settle(o)
	} else {
		lt.armTimer()
	}
	lt.driveAutoFolds(o)
	return nil
}

// driveAutoFolds folds any seat that requested sit_out mid-hand once action
// reaches it. A guard prevents unbounded recursion through applyAction.
func (lt *LocalTable) driveAutoFolds(o *out) {
	if lt.autoFolding {
		return
	}
	lt.autoFolding = true
	defer func() { lt.autoFolding = false }()
	for lt.hand != nil && lt.hand.Street != engine.Showdown {
		seat := lt.toActSeat()
		if seat < 0 {
			return
		}
		s, ok := lt.seats[seat]
		if !ok || !s.foldPending {
			return
		}
		if lt.handIndex(seat) < 0 {
			return
		}
		_ = lt.applyAction(o, seat, engine.Action{SeatID: seat, Kind: engine.Fold}, "fold")
	}
}

// onTurnTimeout auto-acts for the seat on the clock: check if legal, else fold,
// and mark that seat sitting-out for the next hand.
func (lt *LocalTable) onTurnTimeout(o *out) {
	if lt.hand == nil {
		return
	}
	idx := lt.hand.ToActPos
	seat := lt.hand.Players[idx].SeatID
	kind := engine.Fold
	kindStr := "fold"
	if lt.hand.Players[idx].Committed == lt.hand.CurrentBet {
		kind, kindStr = engine.Check, "check"
	}
	if s, ok := lt.seats[seat]; ok {
		s.sittingOut = true
	}
	lt.turnDeadlineMs = 0
	_ = lt.applyAction(o, seat, engine.Action{SeatID: seat, Kind: kind}, kindStr)
}

// settle resolves a hand at showdown: award pots, write stacks back to seats,
// broadcast the showdown and fairness reveal, rotate the button, and try the
// next hand.
func (lt *LocalTable) settle(o *out) {
	lt.turnDeadlineMs = 0
	settled, err := engine.Settle(*lt.hand)
	if err != nil {
		return
	}

	for _, p := range settled.Players {
		if s, ok := lt.seats[p.SeatID]; ok {
			s.stack = p.Stack
			if p.Stack == 0 {
				s.sittingOut = true
			}
		}
	}

	results := map[int]string{}
	revealed := map[int][]string{}
	if settled.Results != nil {
		for seatID, v := range settled.Results {
			results[seatID] = v.Category.String()
		}
		for _, p := range lt.hand.Players {
			if p.Status == engine.Active || p.Status == engine.AllIn {
				revealed[p.SeatID] = []string{p.Hole[0].String(), p.Hole[1].String()}
			}
		}
	}

	o.broadcast(protocol.EvShowdown, showdown{
		HandID:   lt.handID,
		Board:    cardsToStrings(lt.hand.Board),
		Results:  results,
		Awards:   settled.Awards,
		Revealed: revealed,
	})
	o.broadcast(protocol.EvFairReveal, protocol.FairReveal{
		HandID:     lt.handID,
		Commitment: lt.commitment,
		Seed:       lt.seedHex,
	})

	lt.hand = nil
	lt.rotateButton()
	lt.startHandIfReady(o)
}

// rotateButton moves the button to the next occupied seat after the current one.
func (lt *LocalTable) rotateButton() {
	seated := lt.sortedSeatIDs()
	if len(seated) == 0 {
		return
	}
	for _, id := range seated {
		if id > lt.button {
			lt.button = id
			return
		}
	}
	lt.button = seated[0]
}
