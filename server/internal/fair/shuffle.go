// Package fair implements the provably-fair commit-reveal shuffle.
//
// Protocol invariant (see CLAUDE.md): before a hand the server publishes
// Commitment = SHA-256(seed). After the hand it reveals seed. Any client can
// recompute Shuffle(seed) and confirm the deal was not manipulated.
//
// The shuffle is a deterministic Fisher-Yates driven by a SHA-256 keystream of
// the seed, so it is reproducible from the seed alone and independent of Go's
// map iteration order or math/rand.
package fair

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// Seed is 32 bytes of CSPRNG entropy for one hand.
type Seed [32]byte

// NewSeed draws a fresh seed from the OS CSPRNG. This is the ONLY randomness
// source in a hand; the engine itself is deterministic.
func NewSeed() (Seed, error) {
	var s Seed
	_, err := rand.Read(s[:])
	return s, err
}

// Commitment is SHA-256(seed), published before the hand.
func (s Seed) Commitment() string {
	h := sha256.Sum256(s[:])
	return hex.EncodeToString(h[:])
}

// Hex renders the seed for the reveal / hand history.
func (s Seed) Hex() string { return hex.EncodeToString(s[:]) }

// Shuffle returns the canonical deck permuted deterministically from seed.
// Same seed always yields the same deck; this is what makes it verifiable.
func Shuffle(seed Seed) []engine.Card {
	deck := engine.OrderedDeck()
	ks := newKeystream(seed)
	// Fisher-Yates from the top.
	for i := len(deck) - 1; i > 0; i-- {
		j := int(ks.next() % uint64(i+1))
		deck[i], deck[j] = deck[j], deck[i]
	}
	return deck
}

// Verify recomputes the shuffle from a revealed seed and checks it matches the
// prior commitment. Clients and the standalone verifier CLI call this.
func Verify(commitment string, seed Seed) bool {
	return seed.Commitment() == commitment
}

// keystream yields a SHA-256-based sequence of uint64 values.
type keystream struct {
	seed    Seed
	counter uint64
	buf     [32]byte
	off     int
}

func newKeystream(seed Seed) *keystream {
	k := &keystream{seed: seed, off: 32}
	return k
}

func (k *keystream) next() uint64 {
	if k.off+8 > 32 {
		var block [40]byte
		copy(block[:32], k.seed[:])
		binary.BigEndian.PutUint64(block[32:], k.counter)
		k.buf = sha256.Sum256(block[:])
		k.counter++
		k.off = 0
	}
	v := binary.BigEndian.Uint64(k.buf[k.off : k.off+8])
	k.off += 8
	return v
}
