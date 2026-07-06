package economy

import (
	"sync"
	"time"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// PlayerEconomy is the persisted per-player economy record.
type PlayerEconomy struct {
	PlayerID   string
	Balance    engine.Chips
	LastRefill time.Time
	Streak     int
}

// Store is the persistence abstraction for player economy records. The
// in-memory implementation below is a scaffold; a PostgreSQL implementation
// would back Get/Put with row-level locking on the balance to prevent
// double spends across concurrent tables (see economy.go doc comment).
type Store interface {
	// Get returns the record for playerID and whether it exists.
	Get(playerID string) (PlayerEconomy, bool)
	// Put stores (creates or replaces) a player's record. Callers must pass
	// a full, already-computed PlayerEconomy value; Put never mutates it.
	Put(pe PlayerEconomy)
}

// memoryStore is an in-memory, concurrency-safe Store implementation.
type memoryStore struct {
	mu      sync.RWMutex
	records map[string]PlayerEconomy
}

// NewMemoryStore builds an empty in-memory Store.
func NewMemoryStore() Store {
	return &memoryStore{records: map[string]PlayerEconomy{}}
}

func (s *memoryStore) Get(playerID string) (PlayerEconomy, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pe, ok := s.records[playerID]
	return pe, ok
}

func (s *memoryStore) Put(pe PlayerEconomy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[pe.PlayerID] = pe
}
