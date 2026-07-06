// Package social implements the friends graph, online/table presence
// tracking, and the REST surface over both (GitHub issue #18).
//
// It depends on nothing from internal/table or internal/lobby so it can be
// wired in later without those packages importing it back; the presence
// hook (see presence.go) is the seam the orchestrator uses to feed it live
// table state.
package social

import (
	"errors"
	"sync"
)

// Sentinel errors returned by FriendStore methods.
var (
	ErrSelfRequest      = errors.New("social: cannot friend yourself")
	ErrDuplicateRequest = errors.New("social: a pending request already exists")
	ErrAlreadyFriends   = errors.New("social: players are already friends")
	ErrNoSuchRequest    = errors.New("social: no matching pending request")
)

// Request is a pending, directed friend request: From asked to friend To.
type Request struct {
	From string
	To   string
}

// FriendStore manages friend requests and confirmed (symmetric) friendships.
type FriendStore interface {
	// Request creates a pending request from -> to. Errors on self-request,
	// a duplicate pending request in either direction, or if already friends.
	Request(from, to string) error
	// Accept confirms the pending request from -> to, as seen by to. Errors if
	// no such pending request exists.
	Accept(to, from string) error
	// Decline removes the pending request from -> to, as seen by to. Errors if
	// no such pending request exists.
	Decline(to, from string) error
	// Remove unfriends playerID and friendID (symmetric; no-op-safe either way
	// but errors if they were not friends).
	Remove(playerID, friendID string) error
	// FriendsOf lists playerID's confirmed friends.
	FriendsOf(playerID string) []string
	// PendingFor lists incoming pending requests addressed to playerID.
	PendingFor(playerID string) []Request
}

// memFriendStore is a mutex-guarded in-memory FriendStore.
type memFriendStore struct {
	mu sync.Mutex
	// pending maps "from|to" -> Request for O(1) duplicate/lookup checks.
	pending map[string]Request
	// friends maps playerID -> set of friend playerIDs. Kept symmetric: an
	// edge between a and b is stored in both a's and b's sets.
	friends map[string]map[string]bool
}

// NewMemFriendStore builds an empty in-memory FriendStore.
func NewMemFriendStore() FriendStore {
	return &memFriendStore{
		pending: make(map[string]Request),
		friends: make(map[string]map[string]bool),
	}
}

func pendingKey(from, to string) string { return from + "|" + to }

func (s *memFriendStore) areFriendsLocked(a, b string) bool {
	set, ok := s.friends[a]
	return ok && set[b]
}

func (s *memFriendStore) Request(from, to string) error {
	if from == to {
		return ErrSelfRequest
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.areFriendsLocked(from, to) {
		return ErrAlreadyFriends
	}
	if _, ok := s.pending[pendingKey(from, to)]; ok {
		return ErrDuplicateRequest
	}
	if _, ok := s.pending[pendingKey(to, from)]; ok {
		return ErrDuplicateRequest
	}
	s.pending[pendingKey(from, to)] = Request{From: from, To: to}
	return nil
}

func (s *memFriendStore) Accept(to, from string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := pendingKey(from, to)
	if _, ok := s.pending[key]; !ok {
		return ErrNoSuchRequest
	}
	delete(s.pending, key)
	s.addFriendLocked(from, to)
	return nil
}

func (s *memFriendStore) Decline(to, from string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := pendingKey(from, to)
	if _, ok := s.pending[key]; !ok {
		return ErrNoSuchRequest
	}
	delete(s.pending, key)
	return nil
}

func (s *memFriendStore) addFriendLocked(a, b string) {
	if s.friends[a] == nil {
		s.friends[a] = make(map[string]bool)
	}
	if s.friends[b] == nil {
		s.friends[b] = make(map[string]bool)
	}
	s.friends[a][b] = true
	s.friends[b][a] = true
}

func (s *memFriendStore) Remove(playerID, friendID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.areFriendsLocked(playerID, friendID) {
		return ErrNoSuchRequest
	}
	delete(s.friends[playerID], friendID)
	delete(s.friends[friendID], playerID)
	return nil
}

func (s *memFriendStore) FriendsOf(playerID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	set := s.friends[playerID]
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

func (s *memFriendStore) PendingFor(playerID string) []Request {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Request, 0)
	for _, req := range s.pending {
		if req.To == playerID {
			out = append(out, req)
		}
	}
	return out
}
