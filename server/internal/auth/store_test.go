package auth

import "testing"

func TestCreateGuestAllocatesUniquePlayerID(t *testing.T) {
	s := NewMemStore()

	a1 := s.CreateGuest()
	a2 := s.CreateGuest()

	if a1.PlayerID == "" {
		t.Fatal("CreateGuest returned empty PlayerID")
	}
	if a1.PlayerID == a2.PlayerID {
		t.Fatal("CreateGuest returned the same PlayerID twice")
	}
}

func TestUpgradeToAccountPreservesPlayerID(t *testing.T) {
	s := NewMemStore()
	guest := s.CreateGuest()

	acc, err := s.UpgradeToAccount(guest.PlayerID, "chayan@example.com")
	if err != nil {
		t.Fatalf("UpgradeToAccount returned error: %v", err)
	}

	if acc.PlayerID != guest.PlayerID {
		t.Errorf("PlayerID = %q, want %q (must preserve guest identity)", acc.PlayerID, guest.PlayerID)
	}
	if acc.Email != "chayan@example.com" {
		t.Errorf("Email = %q, want %q", acc.Email, "chayan@example.com")
	}

	byID, ok := s.ByID(guest.PlayerID)
	if !ok {
		t.Fatal("ByID did not find upgraded account")
	}
	if byID.Email != "chayan@example.com" {
		t.Errorf("ByID Email = %q, want %q", byID.Email, "chayan@example.com")
	}
}

func TestUpgradeToAccountRejectsDuplicateEmail(t *testing.T) {
	s := NewMemStore()
	guestA := s.CreateGuest()
	guestB := s.CreateGuest()

	if _, err := s.UpgradeToAccount(guestA.PlayerID, "shared@example.com"); err != nil {
		t.Fatalf("first UpgradeToAccount returned error: %v", err)
	}

	if _, err := s.UpgradeToAccount(guestB.PlayerID, "shared@example.com"); err != ErrEmailTaken {
		t.Fatalf("second UpgradeToAccount error = %v, want ErrEmailTaken", err)
	}
}

func TestByEmailFindsUpgradedAccount(t *testing.T) {
	s := NewMemStore()
	guest := s.CreateGuest()
	_, _ = s.UpgradeToAccount(guest.PlayerID, "find-me@example.com")

	acc, ok := s.ByEmail("find-me@example.com")
	if !ok {
		t.Fatal("ByEmail did not find account")
	}
	if acc.PlayerID != guest.PlayerID {
		t.Errorf("PlayerID = %q, want %q", acc.PlayerID, guest.PlayerID)
	}
}

func TestByEmailMissingReturnsFalse(t *testing.T) {
	s := NewMemStore()
	if _, ok := s.ByEmail("nobody@example.com"); ok {
		t.Fatal("ByEmail returned ok=true for unknown email")
	}
}

func TestByIDMissingReturnsFalse(t *testing.T) {
	s := NewMemStore()
	if _, ok := s.ByID("nonexistent"); ok {
		t.Fatal("ByID returned ok=true for unknown id")
	}
}
