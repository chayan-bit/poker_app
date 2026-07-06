package table

import (
	"fmt"
	"sort"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/fair"
	"github.com/chayan-bit/poker_app/server/internal/history"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// eligibleSeats returns the seat IDs that will be dealt into the next hand
// (seated, with chips, not sitting out) in ascending seat order.
func (t *Table) eligibleSeats() []int {
	var out []int
	for id, s := range t.seats {
		if s.stack > 0 && !s.sittingOut {
			out = append(out, id)
		}
	}
	sort.Ints(out)
	return out
}

// startHandIfReady deals a new hand when two or more seats are ready and none is
// already running. It commits a fair seed before dealing (protocol invariant),
// deals per-player hole cards privately, records the hand, and arms the timer.
func (t *Table) startHandIfReady() {
	if t.Hand != nil {
		return
	}
	// A completed tournament never deals again.
	if t.tourneyDone {
		return
	}
	eligible := t.eligibleSeats()
	if len(eligible) < 2 {
		return
	}
	// The first tournament hand waits until every pre-seated player is connected
	// so nobody is dealt in before their client can receive hole cards.
	if t.Cfg.Tournament != nil && t.handNum == 0 && !t.tourneyReadyToStart() {
		return
	}
	// Private rooms wait for the host to deal the first hand; broadcast a status
	// so the host's client can render a start button, then stop here.
	if !t.autoStart() && !t.hostStarted {
		t.broadcastStatus(true)
		return
	}

	seed, err := fair.NewSeed()
	if err != nil {
		return // no CSPRNG -> never deal without a committed seed
	}
	t.commitment = seed.Commitment()
	t.seedHex = seed.Hex()
	deck := fair.Shuffle(seed)

	buttonPos := t.buttonPos(eligible)
	players := make([]engine.Player, len(eligible))
	for i, seat := range eligible {
		players[i] = engine.Player{SeatID: seat, Stack: t.seats[seat].stack, Status: engine.Active}
	}

	hand, err := engine.NewHand(engine.HandConfig{
		Players:    players,
		Deck:       deck,
		ButtonPos:  buttonPos,
		SmallBlind: t.Cfg.SmallBlind,
		BigBlind:   t.Cfg.BigBlind,
	})
	if err != nil {
		return
	}

	t.button = eligible[buttonPos]
	t.handNum++
	t.evictBrokePlayers()
	// A fresh hand carries no pending sit-out folds.
	for _, s := range t.seats {
		s.foldPending = false
	}
	t.handID = fmt.Sprintf("%s-h%d", t.Cfg.ID, t.handNum)
	t.Hand = &hand
	t.recordStart(eligible)
	t.dealHoleCards()
	t.armTimer()
}

// evictBrokePlayers auto-unseats any seat that has been broke (0 chips) for
// brokeUnseatAfterHands hands since it went broke. Called at each hand start, so
// the count only advances when hands actually deal. Broke seats are already
// excluded from the deal (0 stack), so removing them does not affect this hand.
func (t *Table) evictBrokePlayers() {
	changed := false
	for id, s := range t.seats {
		if s.stack == 0 && s.brokeAtHand > 0 && t.handNum-s.brokeAtHand >= brokeUnseatAfterHands {
			t.deps.Ledger.CashOut(s.playerID, 0) // nothing to return; keeps ledger symmetric
			delete(t.seats, id)
			changed = true
		}
	}
	if changed {
		t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
	}
}

// buttonPos returns the index into eligible of the dealer button. It advances to
// the current button seat if still eligible, else the lowest eligible seat.
func (t *Table) buttonPos(eligible []int) int {
	for i, seat := range eligible {
		if seat == t.button {
			return i
		}
	}
	for i, seat := range eligible {
		if seat > t.button {
			return i
		}
	}
	return 0
}

// recordStart begins the durable hand record with pre-deal metadata.
func (t *Table) recordStart(eligible []int) {
	t.startStack = map[int]engine.Chips{}
	seats := make([]history.SeatInfo, 0, len(eligible))
	for _, seat := range eligible {
		t.startStack[seat] = t.seats[seat].stack
		seats = append(seats, history.SeatInfo{
			SeatID: seat, PlayerID: t.seats[seat].playerID, StartStack: t.seats[seat].stack,
		})
	}
	t.rec = history.NewRecorder(
		t.handID, t.Cfg.ID, t.deps.Now(), t.button,
		[2]engine.Chips{t.Cfg.SmallBlind, t.Cfg.BigBlind}, t.commitment, seats,
	)
}

// dealHoleCards sends each dealt-in player only their own two hole cards.
func (t *Table) dealHoleCards() {
	for _, p := range t.Hand.Players {
		s, ok := t.seats[p.SeatID]
		if !ok {
			continue
		}
		out, subscribed := t.subs[s.playerID]
		if !subscribed {
			continue
		}
		t.sendTo(out, protocol.EvHandDealt, protocol.HandDealt{
			TableID:    t.Cfg.ID,
			HandID:     t.handID,
			Commitment: t.commitment,
			YourSeat:   p.SeatID,
			YourHole:   []string{p.Hole[0].String(), p.Hole[1].String()},
			ButtonSeat: t.button,
			Blinds:     [2]int64{int64(t.Cfg.SmallBlind), int64(t.Cfg.BigBlind)},
		})
	}
}

// settle resolves a hand at showdown: award pots, write stacks back to seats,
// broadcast the showdown and the fairness reveal, persist the record, rotate the
// button, and schedule the next hand.
func (t *Table) settle() {
	t.stopTimer()
	settled, err := engine.Settle(*t.Hand)
	if err != nil {
		return
	}

	for _, p := range settled.Players {
		if s, ok := t.seats[p.SeatID]; ok {
			s.stack = p.Stack
			// A busted seat is sat out immediately and starts its broke countdown
			// toward auto-unseat (see evictBrokePlayers).
			if p.Stack == 0 && s.brokeAtHand == 0 {
				s.sittingOut = true
				s.brokeAtHand = t.handNum
			}
		}
	}

	results := map[int]string{}
	revealed := map[int][]string{}
	if settled.Results != nil {
		for seatID, v := range settled.Results {
			results[seatID] = v.Category.String()
		}
		for _, p := range t.Hand.Players {
			if p.Status == engine.Active || p.Status == engine.AllIn {
				revealed[p.SeatID] = []string{p.Hole[0].String(), p.Hole[1].String()}
			}
		}
	}

	t.broadcast(protocol.EvShowdown, showdown{
		HandID:   t.handID,
		Board:    cardsToStrings(t.Hand.Board),
		Results:  results,
		Awards:   settled.Awards,
		Revealed: revealed,
	})
	t.broadcast(protocol.EvFairReveal, protocol.FairReveal{
		HandID:     t.handID,
		Commitment: t.commitment,
		Seed:       t.seedHex,
	})

	if t.rec != nil {
		t.rec.OnShowdown(settled.Awards, results)
		t.rec.OnReveal(t.seedHex)
		_ = t.deps.History.Save(t.rec.Finish())
	}

	t.Hand = nil
	t.rec = nil
	t.rotateButton()
	// Tournament sequencing (blind clock, eliminations, payouts) is decided by
	// the controller in package tourney and applied mechanically here.
	if t.Cfg.Tournament != nil && t.deps.OnHandComplete != nil {
		t.applyTourneyDirective(t.deps.OnHandComplete(t.tourneyStandings()))
	}
	t.startHandIfReady()
}

// rotateButton moves the button to the next occupied seat after the current one.
func (t *Table) rotateButton() {
	var seated []int
	for id := range t.seats {
		seated = append(seated, id)
	}
	if len(seated) == 0 {
		return
	}
	sort.Ints(seated)
	for _, id := range seated {
		if id > t.button {
			t.button = id
			return
		}
	}
	t.button = seated[0]
}
