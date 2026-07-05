package fair

import "testing"

func TestShuffleDeterministic(t *testing.T) {
	var seed Seed
	for i := range seed {
		seed[i] = byte(i)
	}
	a := Shuffle(seed)
	b := Shuffle(seed)
	if len(a) != 52 {
		t.Fatalf("deck size %d", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("shuffle not deterministic at %d", i)
		}
	}
}

func TestShuffleIsPermutation(t *testing.T) {
	seed, _ := NewSeed()
	deck := Shuffle(seed)
	seen := map[int]bool{}
	for _, card := range deck {
		seen[card.Index()] = true
	}
	if len(seen) != 52 {
		t.Fatalf("shuffle lost cards: %d unique", len(seen))
	}
}

func TestCommitReveal(t *testing.T) {
	seed, _ := NewSeed()
	commit := seed.Commitment()
	if !Verify(commit, seed) {
		t.Fatal("valid seed failed verification")
	}
	var tampered Seed = seed
	tampered[0] ^= 0xFF
	if Verify(commit, tampered) {
		t.Fatal("tampered seed passed verification")
	}
}
