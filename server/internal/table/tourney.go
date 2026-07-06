package table

import (
	"sort"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
)

// This file is the single, additive seam that lets a table run as a
// single-table tournament (sit-and-go) WITHOUT any tournament sequencing logic
// living here. All the tournament brains - the blind clock, elimination
// ordering, and payouts - live in package tourney, which supplies one callback
// (Deps.OnHandComplete). The table only does mechanical work: pre-seat the
// registered players at their tournament stack, invoke the callback after each
// hand, and apply the returned directive (raise blinds, remove busted seats,
// stop dealing when done) while emitting the corresponding protocol events.
//
// Every field and branch added for this is guarded by Config.Tournament != nil,
// so a cash table is completely unaffected and all existing behaviour is
// preserved.

// TourneyRules turns a table into a single-table tournament. When set on
// Config, the table pre-seats Seats at StartingStack (tournament chips, NOT a
// ledger buy-in - the buy-in was already collected at registration), runs
// auto-start, and defers blind/elimination/payout sequencing to
// Deps.OnHandComplete.
type TourneyRules struct {
	StartingStack engine.Chips  // tournament chips each seat starts with
	NoRebuy       bool          // reject rebuy while the tournament is live
	Seats         []TourneySeat // initial seating (playerID per seat)
}

// TourneySeat assigns one registered player to a seat.
type TourneySeat struct {
	Seat     int
	PlayerID string
}

// SeatResult is one seat's standing at the end of a hand, handed to
// OnHandComplete so the controller can detect eliminations and order them.
type SeatResult struct {
	Seat       int
	PlayerID   string
	Stack      engine.Chips // chips after the hand settled (0 == busted)
	StartStack engine.Chips // chips at the start of the hand (bust-order tiebreak)
}

// TourneyDirective is what OnHandComplete returns to steer the next hand. The
// table applies it mechanically; it computes none of it.
type TourneyDirective struct {
	SmallBlind    engine.Chips           // blinds for the next hand
	BigBlind      engine.Chips           //
	Level         int                    // 1-based level now in effect
	BlindsChanged bool                   // emit blinds_up when true
	Eliminations  []protocol.Elimination // seats to remove, best finishing place first
	Done          bool                   // tournament over: stop dealing
	Result        *protocol.TourneyResult // final places+prizes (set when Done)
}

// OnHandComplete is invoked from the table goroutine after every settled hand
// in tournament mode, with the current seats' standings. It returns the
// directive that steers the next hand.
type OnHandComplete func(standings []SeatResult) TourneyDirective

// seatTournamentPlayers pre-seats the tournament's registered players at their
// starting stack. Called from New before the loop starts, so it is the sole
// writer. Cash tables never call this.
func (t *Table) seatTournamentPlayers() {
	for _, ts := range t.Cfg.Tournament.Seats {
		t.seats[ts.Seat] = &seatState{
			playerID: ts.PlayerID,
			stack:    t.Cfg.Tournament.StartingStack,
		}
	}
}

// tourneyReadyToStart gates the FIRST tournament hand until every pre-seated
// player has a live subscription, so nobody is dealt in before their client is
// connected to receive hole cards. After the first hand it is irrelevant.
func (t *Table) tourneyReadyToStart() bool {
	for _, s := range t.seats {
		if _, ok := t.subs[s.playerID]; !ok {
			return false
		}
	}
	return true
}

// tourneyStandings snapshots each current seat's post-hand and start-of-hand
// stacks for the controller.
func (t *Table) tourneyStandings() []SeatResult {
	out := make([]SeatResult, 0, len(t.seats))
	for id, s := range t.seats {
		out = append(out, SeatResult{
			Seat:       id,
			PlayerID:   s.playerID,
			Stack:      s.stack,
			StartStack: t.startStack[id],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seat < out[j].Seat })
	return out
}

// applyTourneyDirective enacts the controller's decision: raise blinds, remove
// busted seats, and, when the tournament is over, publish the result and stop
// dealing. Runs on the table goroutine (from settle), so it is the sole writer.
func (t *Table) applyTourneyDirective(d TourneyDirective) {
	if d.SmallBlind > 0 && d.BigBlind > 0 {
		t.Cfg.SmallBlind = d.SmallBlind
		t.Cfg.BigBlind = d.BigBlind
	}
	if d.BlindsChanged {
		t.broadcast(protocol.EvBlindsUp, protocol.BlindsUp{
			Level: d.Level, SB: int64(t.Cfg.SmallBlind), BB: int64(t.Cfg.BigBlind),
		})
	}
	for _, e := range d.Eliminations {
		delete(t.seats, e.Seat)
		t.broadcast(protocol.EvElimination, e)
	}
	if len(d.Eliminations) > 0 {
		t.broadcast(protocol.EvSeatUpdate, t.seatUpdate())
	}
	if d.Done {
		t.tourneyDone = true
		if d.Result != nil {
			t.broadcast(protocol.EvTourneyResult, *d.Result)
		}
	}
}
