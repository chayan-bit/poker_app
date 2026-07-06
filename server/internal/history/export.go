package history

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chayan-bit/poker_app/server/internal/engine"
)

// ExportText renders a HandRecord as a PokerStars-style plain-text hand
// history: header, seats with starting stacks, per-street actions with the
// board as it develops, and a summary including pot awards, shown hands, and
// the fairness commit/reveal pair.
func ExportText(rec HandRecord) string {
	var b strings.Builder

	fmt.Fprintf(&b, "PokerApp Hand #%s: Table '%s' (%d/%d)\n",
		rec.HandID, rec.TableID, rec.Blinds[0], rec.Blinds[1])
	fmt.Fprintf(&b, "Hand started at %s\n", rec.StartedAt.UTC().Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(&b, "Table '%s' Seat #%d is the button\n", rec.TableID, rec.ButtonSeat)

	for _, seat := range rec.Seats {
		fmt.Fprintf(&b, "Seat %d: %s (%d in chips)\n", seat.SeatID, seat.PlayerID, seat.StartStack)
	}

	currentStreet := ""
	boardIdx := 0
	for _, ev := range rec.Events {
		if ev.Kind == "street" {
			currentStreet = ev.Street
			cards := streetCardCount(ev.Street)
			shown := rec.Board
			if boardIdx+cards <= len(rec.Board) {
				shown = rec.Board[:boardIdx+cards]
			}
			boardIdx += cards
			fmt.Fprintf(&b, "*** %s *** [%s]\n", strings.ToUpper(ev.Street), strings.Join(shown, " "))
			continue
		}
		fmt.Fprintf(&b, "Seat %d: %s%s\n", ev.SeatID, ev.Kind, amountSuffix(ev.Amount))
		_ = currentStreet
	}

	b.WriteString("*** SUMMARY ***\n")
	if len(rec.Board) > 0 {
		fmt.Fprintf(&b, "Board [%s]\n", strings.Join(rec.Board, " "))
	}
	for _, award := range rec.Awards {
		fmt.Fprintf(&b, "Seat %d collected %d from pot\n", award.SeatID, award.Amount)
	}
	for _, seat := range rec.Seats {
		if desc, ok := rec.Results[seat.SeatID]; ok {
			fmt.Fprintf(&b, "Seat %d: %s\n", seat.SeatID, desc)
		}
	}

	fmt.Fprintf(&b, "Fairness commitment: %s\n", rec.Commitment)
	fmt.Fprintf(&b, "Fairness seed: %s\n", rec.SeedHex)

	return b.String()
}

func streetCardCount(street string) int {
	switch strings.ToLower(street) {
	case "flop":
		return 3
	case "turn", "river":
		return 1
	default:
		return 0
	}
}

func amountSuffix(amount engine.Chips) string {
	if amount == 0 {
		return ""
	}
	return fmt.Sprintf(" %d", amount)
}

// ExportJSON serializes a HandRecord as the payload backing a shareable
// replay link.
func ExportJSON(rec HandRecord) ([]byte, error) {
	return json.Marshal(rec)
}
