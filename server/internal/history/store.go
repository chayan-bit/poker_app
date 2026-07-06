package history

import "sync"

// Store persists and retrieves completed HandRecords.
type Store interface {
	// Save persists a HandRecord, keyed by its HandID.
	Save(rec HandRecord) error
	// Get retrieves a HandRecord by HandID.
	Get(handID string) (HandRecord, bool)
	// ByPlayer returns up to limit HandRecords that include playerID as a
	// seat, most-recent-first (by StartedAt). limit <= 0 means no limit.
	ByPlayer(playerID string, limit int) []HandRecord
}

// memStore is an in-memory Store, safe for concurrent use.
type memStore struct {
	mu     sync.Mutex
	byHand map[string]HandRecord
	order  []string // HandIDs in insertion order, for stable iteration
}

// NewMemStore returns a mutex-guarded in-memory Store implementation.
func NewMemStore() Store {
	return &memStore{
		byHand: make(map[string]HandRecord),
	}
}

func (s *memStore) Save(rec HandRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byHand[rec.HandID]; !exists {
		s.order = append(s.order, rec.HandID)
	}
	s.byHand[rec.HandID] = rec
	return nil
}

func (s *memStore) Get(handID string) (HandRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byHand[handID]
	return rec, ok
}

func (s *memStore) ByPlayer(playerID string, limit int) []HandRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	var matches []HandRecord
	for i := len(s.order) - 1; i >= 0; i-- {
		rec := s.byHand[s.order[i]]
		if recordHasPlayer(rec, playerID) {
			matches = append(matches, rec)
		}
	}

	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func recordHasPlayer(rec HandRecord, playerID string) bool {
	for _, seat := range rec.Seats {
		if seat.PlayerID == playerID {
			return true
		}
	}
	return false
}
