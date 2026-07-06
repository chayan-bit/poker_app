package tourney

import (
	"sort"
	"sync"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/protocol"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// Status is the lifecycle phase of a sit-and-go.
type Status int

const (
	Registering Status = iota // accepting registrations; table not yet created
	Running                   // full: table created and dealing
	Complete                  // one player left; prizes paid
)

// finisher records one player's final standing as it is decided. batch groups
// players eliminated in the same hand (0 == the eventual winner); startStack is
// the tie key that both orders simultaneous busts and detects genuine ties.
type finisher struct {
	playerID   string
	place      int
	batch      int
	startStack engine.Chips
}

// SNG is one sit-and-go: the wrapper around a single tournament table. All of
// its mutable state is guarded by mu, which is taken both by REST-driven
// registration/listing and by the OnHandComplete callback (invoked on the
// table goroutine), so the two never race.
type SNG struct {
	mu sync.Mutex

	ID      string
	TableID string
	Cfg     SNGConfig

	now func() time.Time

	status     Status
	registered []string     // playerIDs, in registration (seat) order
	prizePool  engine.Chips // sum of buy-ins collected

	// running-phase controller state (written only under mu)
	startTime  time.Time
	curLevel   int // 0-based index into Cfg.BlindLevels
	eliminated int // players eliminated so far
	batch      int // elimination-hand counter (for tie grouping)
	finishers  []finisher

	ledger Ledger
	tbl    *table.Table // set at auto-start
}

// View is the public listing shape for GET /api/sng.
type View struct {
	SngID      string       `json:"sngId"`
	Name       string       `json:"name"`
	Seats      int          `json:"seats"`
	Registered int          `json:"registered"`
	BuyIn      engine.Chips `json:"buyIn"`
}

// view renders a listing entry (caller holds mu).
func (s *SNG) view() View {
	return View{
		SngID:      s.ID,
		Name:       s.Cfg.Name,
		Seats:      s.Cfg.Seats,
		Registered: len(s.registered),
		BuyIn:      s.Cfg.BuyIn,
	}
}

// controller returns the OnHandComplete callback the table invokes after every
// settled hand. It simply guards onHandComplete with the SNG mutex.
func (s *SNG) controller() table.OnHandComplete {
	return func(standings []table.SeatResult) table.TourneyDirective {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.onHandComplete(standings)
	}
}

// onHandComplete is the tournament brain (caller holds mu): advance the blind
// clock, assign finishing places to any busted seats, and, when one player
// remains, pay out the prize pool.
func (s *SNG) onHandComplete(standings []table.SeatResult) table.TourneyDirective {
	var d table.TourneyDirective

	// 1. Blind clock: the level is a pure function of elapsed time. Because this
	// runs only between hands, a raise always applies at the next hand start.
	lvl := s.levelFor(s.now().Sub(s.startTime))
	if lvl != s.curLevel {
		d.BlindsChanged = true
		s.curLevel = lvl
	}
	d.SmallBlind = s.Cfg.BlindLevels[lvl].SmallBlind
	d.BigBlind = s.Cfg.BlindLevels[lvl].BigBlind
	d.Level = lvl + 1

	// 2. Eliminations: any seat at 0 chips busted this hand.
	var busted, alive []table.SeatResult
	for _, st := range standings {
		if st.Stack <= 0 {
			busted = append(busted, st)
		} else {
			alive = append(alive, st)
		}
	}
	if len(busted) > 0 {
		s.batch++
		// Simultaneous busts: the bigger start-of-hand stack finishes higher
		// (better = lower place number); identical stacks are a genuine tie,
		// ordered by seat for determinism and split at payout time.
		sort.Slice(busted, func(i, j int) bool {
			if busted[i].StartStack != busted[j].StartStack {
				return busted[i].StartStack > busted[j].StartStack
			}
			return busted[i].Seat < busted[j].Seat
		})
		worstPlace := s.Cfg.Seats - s.eliminated
		bestPlace := worstPlace - len(busted) + 1
		for i, b := range busted {
			place := bestPlace + i
			s.finishers = append(s.finishers, finisher{b.PlayerID, place, s.batch, b.StartStack})
			d.Eliminations = append(d.Eliminations, protocol.Elimination{
				Seat: b.Seat, PlayerID: b.PlayerID, Place: place,
			})
		}
		s.eliminated += len(busted)
	}

	// 3. Completion: one (or zero) players left. Assign the winner place 1 and
	// distribute the prize pool.
	if len(alive) <= 1 {
		d.Done = true
		s.status = Complete
		if len(alive) == 1 {
			w := alive[0]
			s.finishers = append(s.finishers, finisher{w.PlayerID, 1, 0, w.StartStack})
		}
		d.Result = s.payout()
	}
	return d
}

// levelFor returns the 0-based blind level for the elapsed tournament time,
// capped at the final level (blinds never run past the schedule).
func (s *SNG) levelFor(elapsed time.Duration) int {
	var acc time.Duration
	for i, lv := range s.Cfg.BlindLevels {
		acc += lv.Duration
		if elapsed < acc {
			return i
		}
	}
	return len(s.Cfg.BlindLevels) - 1
}

// payout computes and credits prizes (caller holds mu). Rules:
//   - Place prizes are prizePool * pct / 100, with any rounding remainder added
//     to first place so the pool is fully paid.
//   - Players who tied (busted the same hand with identical start stacks) pool
//     the prizes of the places they occupy and split them evenly; any leftover
//     chip goes to the earlier (higher-placed) finisher.
func (s *SNG) payout() *protocol.TourneyResult {
	prizes := computePrizes(s.prizePool, s.Cfg.PayoutPct)
	prizeFor := func(place int) engine.Chips {
		if place >= 1 && place-1 < len(prizes) {
			return prizes[place-1]
		}
		return 0
	}

	type key struct {
		batch int
		stack engine.Chips
	}
	groups := map[key][]finisher{}
	var order []key
	for _, f := range s.finishers {
		k := key{f.batch, f.startStack}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], f)
	}

	payByPlayer := map[string]engine.Chips{}
	for _, k := range order {
		members := groups[k]
		sort.Slice(members, func(i, j int) bool { return members[i].place < members[j].place })
		var total engine.Chips
		for _, m := range members {
			total += prizeFor(m.place)
		}
		n := engine.Chips(len(members))
		share := total / n
		rem := total - share*n
		for i, m := range members {
			amt := share
			if engine.Chips(i) < rem {
				amt++ // leftover chips to the earlier finishers
			}
			payByPlayer[m.playerID] = amt
		}
	}

	fs := append([]finisher(nil), s.finishers...)
	sort.Slice(fs, func(i, j int) bool { return fs[i].place < fs[j].place })
	places := make([]protocol.TourneyPlace, 0, len(fs))
	for _, f := range fs {
		prize := payByPlayer[f.playerID]
		if prize > 0 {
			s.ledger.Credit(f.playerID, prize)
		}
		places = append(places, protocol.TourneyPlace{
			PlayerID: f.playerID, Place: f.place, Prize: int64(prize),
		})
	}
	return &protocol.TourneyResult{Places: places}
}

// computePrizes splits pool across places by percentage, giving any rounding
// remainder to first place so the whole pool is always paid out.
func computePrizes(pool engine.Chips, pct []int) []engine.Chips {
	out := make([]engine.Chips, len(pct))
	var assigned engine.Chips
	for i, p := range pct {
		out[i] = pool * engine.Chips(p) / 100
		assigned += out[i]
	}
	if len(out) > 0 {
		out[0] += pool - assigned
	}
	return out
}
