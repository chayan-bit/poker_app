package social

import "testing"

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}

func TestRequestAcceptLifecycle(t *testing.T) {
	s := NewMemFriendStore()
	if err := s.Request("alice", "bob"); err != nil {
		t.Fatalf("Request: %v", err)
	}
	pending := s.PendingFor("bob")
	if len(pending) != 1 || pending[0].From != "alice" || pending[0].To != "bob" {
		t.Fatalf("PendingFor(bob) = %+v, want one request from alice", pending)
	}
	if err := s.Accept("bob", "alice"); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if !contains(s.FriendsOf("alice"), "bob") {
		t.Fatalf("FriendsOf(alice) should contain bob after accept")
	}
	if !contains(s.FriendsOf("bob"), "alice") {
		t.Fatalf("FriendsOf(bob) should contain alice after accept (symmetric)")
	}
	if len(s.PendingFor("bob")) != 0 {
		t.Fatalf("pending request should be consumed after accept")
	}
}

func TestRequestDeclineLifecycle(t *testing.T) {
	s := NewMemFriendStore()
	if err := s.Request("alice", "bob"); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if err := s.Decline("bob", "alice"); err != nil {
		t.Fatalf("Decline: %v", err)
	}
	if len(s.PendingFor("bob")) != 0 {
		t.Fatalf("pending request should be removed after decline")
	}
	if contains(s.FriendsOf("alice"), "bob") {
		t.Fatalf("declined request must not create a friendship")
	}
	// Declined request can be re-sent.
	if err := s.Request("alice", "bob"); err != nil {
		t.Fatalf("Request after decline: %v", err)
	}
}

func TestRemove(t *testing.T) {
	s := NewMemFriendStore()
	_ = s.Request("alice", "bob")
	_ = s.Accept("bob", "alice")

	if err := s.Remove("alice", "bob"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if contains(s.FriendsOf("alice"), "bob") || contains(s.FriendsOf("bob"), "alice") {
		t.Fatalf("Remove should unfriend both directions")
	}
	if err := s.Remove("alice", "bob"); err != ErrNoSuchRequest {
		t.Fatalf("Remove on non-friends = %v, want ErrNoSuchRequest", err)
	}
}

func TestSelfRequestRejected(t *testing.T) {
	s := NewMemFriendStore()
	if err := s.Request("alice", "alice"); err != ErrSelfRequest {
		t.Fatalf("Request(self) = %v, want ErrSelfRequest", err)
	}
}

func TestDuplicateRequestRejected(t *testing.T) {
	s := NewMemFriendStore()
	if err := s.Request("alice", "bob"); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if err := s.Request("alice", "bob"); err != ErrDuplicateRequest {
		t.Fatalf("duplicate Request (same direction) = %v, want ErrDuplicateRequest", err)
	}
	if err := s.Request("bob", "alice"); err != ErrDuplicateRequest {
		t.Fatalf("duplicate Request (reverse direction) = %v, want ErrDuplicateRequest", err)
	}
}

func TestRequestWhenAlreadyFriendsRejected(t *testing.T) {
	s := NewMemFriendStore()
	_ = s.Request("alice", "bob")
	_ = s.Accept("bob", "alice")

	if err := s.Request("alice", "bob"); err != ErrAlreadyFriends {
		t.Fatalf("Request between friends = %v, want ErrAlreadyFriends", err)
	}
	if err := s.Request("bob", "alice"); err != ErrAlreadyFriends {
		t.Fatalf("Request between friends (reverse) = %v, want ErrAlreadyFriends", err)
	}
}

func TestAcceptDeclineNoSuchRequest(t *testing.T) {
	s := NewMemFriendStore()
	if err := s.Accept("bob", "alice"); err != ErrNoSuchRequest {
		t.Fatalf("Accept with no pending request = %v, want ErrNoSuchRequest", err)
	}
	if err := s.Decline("bob", "alice"); err != ErrNoSuchRequest {
		t.Fatalf("Decline with no pending request = %v, want ErrNoSuchRequest", err)
	}
}

func TestFriendsOfAndPendingForMultiple(t *testing.T) {
	s := NewMemFriendStore()
	_ = s.Request("alice", "bob")
	_ = s.Request("carol", "bob")
	_ = s.Accept("bob", "alice")

	friends := s.FriendsOf("bob")
	if len(friends) != 1 || !contains(friends, "alice") {
		t.Fatalf("FriendsOf(bob) = %v, want [alice]", friends)
	}
	pending := s.PendingFor("bob")
	if len(pending) != 1 || pending[0].From != "carol" {
		t.Fatalf("PendingFor(bob) = %+v, want one request from carol", pending)
	}
}
