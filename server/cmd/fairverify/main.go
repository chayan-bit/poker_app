// Command fairverify is a standalone CLI that lets any player independently
// verify a poker_app hand's provably-fair shuffle: given the commitment
// published before the hand and the seed revealed after it, it recomputes
// the deck and confirms the deal was not manipulated.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/fair"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run contains all CLI logic so it can be exercised by tests without
// touching the real process args or exit code.
func run(args []string, stdout, stderr io.Writer) (exitCode int) {
	fs := flag.NewFlagSet("fairverify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	commitment := fs.String("commitment", "", "hex-encoded SHA-256 commitment published before the hand")
	seedHex := fs.String("seed", "", "64-character hex seed revealed after the hand")
	asJSON := fs.Bool("json", false, "emit machine-readable JSON instead of human-readable output")

	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *commitment == "" || *seedHex == "" {
		fmt.Fprintln(stderr, "fairverify: --commitment and --seed are required")
		fs.Usage()
		return 2
	}

	seed, err := fair.SeedFromHex(*seedHex)
	if err != nil {
		fmt.Fprintf(stderr, "fairverify: invalid seed: %v\n", err)
		return 2
	}

	if !fair.Verify(*commitment, seed) {
		if *asJSON {
			fmt.Fprintf(stdout, "{\"valid\":false,\"commitment\":%q,\"seed\":%q}\n", *commitment, seed.Hex())
			return 1
		}
		fmt.Fprintln(stderr, "MISMATCH: the revealed seed does not match the published commitment.")
		fmt.Fprintln(stderr, "The hand cannot be verified as fair with these inputs.")
		return 1
	}

	deck := fair.Shuffle(seed)

	if *asJSON {
		writeJSON(stdout, *commitment, seed, deck)
	} else {
		writeHuman(stdout, *commitment, seed, deck)
	}
	return 0
}

// writeHuman prints the verified deck order plus a plain-language
// explanation of how this engine deals from it.
func writeHuman(w io.Writer, commitment string, seed fair.Seed, deck []engine.Card) {
	fmt.Fprintln(w, "VALID: revealed seed matches the published commitment.")
	fmt.Fprintf(w, "commitment: %s\n", commitment)
	fmt.Fprintf(w, "seed:       %s\n", seed.Hex())
	fmt.Fprintf(w, "deck:       %s\n", strings.Join(deckStrings(deck), " "))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Deal order: hole cards are dealt round-robin, one card per player per")
	fmt.Fprintln(w, "round, from the front of this deck (first N*2 cards for an N-handed hand,")
	fmt.Fprintln(w, "in seat order). This engine deals sequentially and does NOT burn cards")
	fmt.Fprintln(w, "before community streets; the next cards in the deck after the hole cards")
	fmt.Fprintln(w, "are dealt straight to the board (flop/turn/river) with no cards skipped.")
}

// writeJSON emits the machine-readable verification result.
func writeJSON(w io.Writer, commitment string, seed fair.Seed, deck []engine.Card) {
	cards := deckStrings(deck)
	quoted := make([]string, len(cards))
	for i, c := range cards {
		quoted[i] = fmt.Sprintf("%q", c)
	}
	fmt.Fprintf(w, "{\"valid\":true,\"commitment\":%q,\"seed\":%q,\"deck\":[%s]}\n",
		commitment, seed.Hex(), strings.Join(quoted, ","))
}

// deckStrings renders each card via its String method (e.g. "As", "Td").
func deckStrings(deck []engine.Card) []string {
	out := make([]string, len(deck))
	for i, c := range deck {
		out[i] = c.String()
	}
	return out
}
