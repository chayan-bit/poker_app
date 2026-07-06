package tourney

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// Ledger is the subset of economy.Ledger the tourney package needs: debit a
// buy-in at registration, credit a prize at payout.
type Ledger interface {
	BuyIn(playerID string, amt engine.Chips) error
	Credit(playerID string, amt engine.Chips)
}

// TableFactory creates a tournament-mode table. table.Registry satisfies it via
// CreateTourney; tests supply a fake. Kept narrow so upstream table signature
// drift is absorbed at the main.go wiring point only.
type TableFactory interface {
	CreateTourney(cfg table.Config, onComplete table.OnHandComplete) *table.Table
}

// Sentinel errors surfaced to the lobby layer for HTTP status mapping.
var (
	ErrNotFound          = errors.New("tourney: no sit-and-go with that id")
	ErrFull              = errors.New("tourney: sit-and-go registration is closed")
	ErrAlreadyRegistered = errors.New("tourney: player already registered")
)

// Reaping TTLs bound how long dead sit-and-gos linger in memory. A never-filled
// (still Registering) SNG is torn down after registerTTL, refunding its
// registrants; a finished (Complete) SNG is dropped after completeTTL.
const (
	registerTTL = 1 * time.Hour
	completeTTL = 10 * time.Minute
)

// Manager owns all live sit-and-gos and their creation/registration.
type Manager struct {
	mu      sync.Mutex
	sngs    map[string]*SNG
	ledger  Ledger
	factory TableFactory
	now     func() time.Time
	newID   func() (string, error)
}

// NewManager builds a production Manager backed by the shared ledger and a
// tournament-capable table factory (the registry).
func NewManager(ledger Ledger, factory TableFactory) *Manager {
	return &Manager{
		sngs:    map[string]*SNG{},
		ledger:  ledger,
		factory: factory,
		now:     time.Now,
		newID:   newID,
	}
}

// Create opens a new sit-and-go in the Registering phase. Both the SNG id and
// its future table id are allocated now, so the lobby can return the table id
// immediately even though the table itself is created only once the SNG fills.
func (m *Manager) Create(cfg SNGConfig) (sngID, tableID string, err error) {
	if err := cfg.validate(); err != nil {
		return "", "", err
	}
	sid, err := m.newID()
	if err != nil {
		return "", "", err
	}
	tid, err := m.newID()
	if err != nil {
		return "", "", err
	}
	s := &SNG{
		ID:        sid,
		TableID:   tid,
		Cfg:       cfg,
		now:       m.now,
		status:    Registering,
		ledger:    m.ledger,
		createdAt: m.now(),
	}
	m.reap()
	m.mu.Lock()
	m.sngs[sid] = s
	m.mu.Unlock()
	return sid, tid, nil
}

// reap removes dead sit-and-gos so the registry does not grow unboundedly: a
// never-filled Registering SNG past registerTTL is torn down and its collected
// buy-ins refunded, and a Complete SNG past completeTTL is dropped (its prizes
// were already paid). Called opportunistically from Create/Register/List.
func (m *Manager) reap() {
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sngs {
		s.mu.Lock()
		switch s.status {
		case Registering:
			if now.Sub(s.createdAt) > registerTTL {
				for _, pid := range s.registered {
					s.ledger.Credit(pid, s.Cfg.BuyIn)
				}
				s.registered = nil
				delete(m.sngs, id)
			}
		case Complete:
			if !s.completedAt.IsZero() && now.Sub(s.completedAt) > completeTTL {
				delete(m.sngs, id)
			}
		}
		s.mu.Unlock()
	}
}

// Shutdown refunds the collected buy-ins of every sit-and-go that has not paid
// out yet (Registering or Running) and clears the registry. Wire it into
// process shutdown so a restart does not strand players' buy-ins. Complete
// sit-and-gos already paid their prizes and are only dropped. Idempotent.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sngs {
		s.mu.Lock()
		if s.status != Complete {
			for _, pid := range s.registered {
				s.ledger.Credit(pid, s.Cfg.BuyIn)
			}
			s.registered = nil
		}
		s.mu.Unlock()
		delete(m.sngs, id)
	}
}

// Register signs playerID up for the sit-and-go, collecting the buy-in. When the
// final seat registers, the tournament auto-starts (its table is created). It
// returns economy.ErrInsufficientFunds when the ledger rejects the buy-in.
func (m *Manager) Register(sngID, playerID string) error {
	m.reap()
	m.mu.Lock()
	s, ok := m.sngs[sngID]
	m.mu.Unlock()
	if !ok {
		return ErrNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status != Registering {
		return ErrFull
	}
	for _, p := range s.registered {
		if p == playerID {
			return ErrAlreadyRegistered
		}
	}
	if err := s.ledger.BuyIn(playerID, s.Cfg.BuyIn); err != nil {
		return err // economy.ErrInsufficientFunds passes through
	}
	s.registered = append(s.registered, playerID)
	s.prizePool += s.Cfg.BuyIn
	if len(s.registered) == s.Cfg.Seats {
		s.autoStart(m.factory)
	}
	return nil
}

// autoStart creates the tournament table and flips the SNG to Running (caller
// holds s.mu). Seats are assigned in registration order.
func (s *SNG) autoStart(factory TableFactory) {
	seats := make([]table.TourneySeat, len(s.registered))
	for i, pid := range s.registered {
		seats[i] = table.TourneySeat{Seat: i, PlayerID: pid}
	}
	cfg := table.Config{
		ID:         s.TableID,
		Visibility: table.Private,
		MaxSeats:   s.Cfg.Seats,
		AutoStart:  true,
		SmallBlind: s.Cfg.BlindLevels[0].SmallBlind,
		BigBlind:   s.Cfg.BlindLevels[0].BigBlind,
		Tournament: &table.TourneyRules{
			StartingStack: s.Cfg.StartingStack,
			NoRebuy:       true,
			Seats:         seats,
		},
	}
	s.startTime = s.now()
	s.curLevel = 0
	s.tbl = factory.CreateTourney(cfg, s.controller())
	s.status = Running
}

// List returns every sit-and-go still open for registration, most-recent id
// order stable across calls.
func (m *Manager) List() []View {
	m.reap()
	m.mu.Lock()
	snapshot := make([]*SNG, 0, len(m.sngs))
	for _, s := range m.sngs {
		snapshot = append(snapshot, s)
	}
	m.mu.Unlock()

	out := make([]View, 0, len(snapshot))
	for _, s := range snapshot {
		s.mu.Lock()
		open := s.status == Registering
		v := s.view()
		s.mu.Unlock()
		if open {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SngID < out[j].SngID })
	return out
}

// get returns an SNG by id (test/introspection helper).
func (m *Manager) get(sngID string) (*SNG, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sngs[sngID]
	return s, ok
}

// newID generates a random opaque identifier (128 bits, hyphenated) using the
// CSPRNG, matching the lobby's table-id format.
func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}

// Ensure economy.Ledger satisfies Ledger at compile time (documents the wiring
// in main.go without importing it there).
var _ Ledger = (*economy.Ledger)(nil)

// Ensure table.Registry satisfies TableFactory at compile time.
var _ TableFactory = (*table.Registry)(nil)
