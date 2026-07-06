package fair

import (
	"strings"
	"testing"
)

func TestSeedFromHexValid(t *testing.T) {
	seed, err := NewSeed()
	if err != nil {
		t.Fatalf("NewSeed: %v", err)
	}
	parsed, err := SeedFromHex(seed.Hex())
	if err != nil {
		t.Fatalf("SeedFromHex: %v", err)
	}
	if parsed != seed {
		t.Fatalf("round-trip mismatch: got %x, want %x", parsed, seed)
	}
}

func TestSeedFromHexWrongLength(t *testing.T) {
	cases := []string{
		"",
		"ab",
		strings.Repeat("a", 62),
		strings.Repeat("a", 66),
	}
	for _, s := range cases {
		if _, err := SeedFromHex(s); err == nil {
			t.Errorf("SeedFromHex(len=%d): expected error for wrong length, got nil", len(s))
		}
	}
}

func TestSeedFromHexNonHex(t *testing.T) {
	// Exactly 64 chars (correct length) but contains non-hex characters.
	s := "zz" + strings.Repeat("a", 62)
	if len(s) != 64 {
		t.Fatalf("test fixture wrong length: %d", len(s))
	}
	if _, err := SeedFromHex(s); err == nil {
		t.Fatal("SeedFromHex: expected error for non-hex input, got nil")
	}
}
