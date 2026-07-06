package auth

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

var ErrEmailTaken = errors.New("email already registered")

// Account is a resolved player identity: either a guest (Email empty) or an
// upgraded account.
type Account struct {
	PlayerID  string
	Email     string
	CreatedAt time.Time
}

// Store manages accounts. Guests get a PlayerID immediately; upgrading to a
// full account keeps that same PlayerID so chips and history carry over.
type Store interface {
	CreateGuest() Account
	UpgradeToAccount(playerID, email string) (Account, error)
	ByEmail(email string) (Account, bool)
	ByID(id string) (Account, bool)
}

// MemStore is an in-memory Store. Swap for PostgreSQL before production.
type MemStore struct {
	mu        sync.Mutex
	byID      map[string]Account
	emailToID map[string]string
}

// NewMemStore builds an empty in-memory store.
func NewMemStore() *MemStore {
	return &MemStore{
		byID:      make(map[string]Account),
		emailToID: make(map[string]string),
	}
}

// CreateGuest allocates a new guest account with a fresh PlayerID.
func (s *MemStore) CreateGuest() Account {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc := Account{
		PlayerID:  uuid.NewString(),
		CreatedAt: time.Now(),
	}
	s.byID[acc.PlayerID] = acc
	return acc
}

// UpgradeToAccount attaches email to the existing playerID, preserving it.
func (s *MemStore) UpgradeToAccount(playerID, email string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, taken := s.emailToID[email]; taken && existingID != playerID {
		return Account{}, ErrEmailTaken
	}

	acc, ok := s.byID[playerID]
	if !ok {
		acc = Account{PlayerID: playerID, CreatedAt: time.Now()}
	}
	acc.Email = email
	s.byID[playerID] = acc
	s.emailToID[email] = playerID
	return acc, nil
}

// ByEmail looks up an account by email.
func (s *MemStore) ByEmail(email string) (Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id, ok := s.emailToID[email]
	if !ok {
		return Account{}, false
	}
	acc, ok := s.byID[id]
	return acc, ok
}

// ByID looks up an account by player ID.
func (s *MemStore) ByID(id string) (Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	acc, ok := s.byID[id]
	return acc, ok
}
