// Package handsapi exposes the read-only REST surface over durable hand
// history: fetching a single hand's full record, a player's recent hand
// summaries, and a plain-text export (Design_suite section 7).
//
// It depends only on the narrow history.Store interface, so it is decoupled
// from however the recorder/table packages evolve.
package handsapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chayan-bit/poker_app/server/internal/history"
)

// AuthFunc resolves the caller's identity from the request, exactly like
// (*auth.Authenticator).FromRequest.
type AuthFunc func(*http.Request) (string, error)

// Handlers holds the hand-history read endpoints.
type Handlers struct {
	store history.Store
	auth  AuthFunc
}

// New builds Handlers backed by store for records and auth for identity.
func New(store history.Store, auth AuthFunc) *Handlers {
	return &Handlers{store: store, auth: auth}
}

// Register wires all hand-history routes onto mux using Go 1.22+ method+path
// patterns. The caller (main.go) owns mux construction; Register never
// touches anything outside its own routes.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/hands/{id}", h.getHand)
	mux.HandleFunc("GET /api/hands/{id}/text", h.getHandText)
	mux.HandleFunc("GET /api/players/me/hands", h.listMyHands)
}

// --- error envelope, matching internal/lobby's style ---

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: apiError{Code: code, Message: message}})
}

// requireAuth resolves identity or writes a 401 and returns ok=false.
func (h *Handlers) requireAuth(w http.ResponseWriter, r *http.Request) (playerID string, ok bool) {
	playerID, err := h.auth(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid auth token")
		return "", false
	}
	return playerID, true
}

// getHand handles GET /api/hands/{id}: the HandRecord as stored, except hole
// cards are masked per requester (see maskHoleCards). Fairness
// commitment/seed fields are always returned as-is, since anyone can already
// recompute the shuffle from them.
func (h *Handlers) getHand(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	rec, ok := h.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no hand with that id")
		return
	}
	masked := maskHoleCards(rec, playerID)
	b, err := history.ExportJSON(masked)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to serialize hand")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

// getHandText handles GET /api/hands/{id}/text: a plain-text hand history,
// with a hole-cards section masked per requester (see maskHoleCards).
func (h *Handlers) getHandText(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	rec, ok := h.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "no hand with that id")
		return
	}
	masked := maskHoleCards(rec, playerID)
	text := history.ExportText(masked) + holeCardsSection(masked)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(text))
}

// maskedHole is the placeholder returned for a seat the requester may not
// see, matching the API contract (["??","??"] in JSON, "(mucked)" in text).
var maskedHole = []string{"??", "??"}

// reachedShowdown reports whether seatID has a recorded showdown result.
// history.Recorder.OnShowdown only populates HandRecord.Results for seats
// that were still Active/AllIn when settle() ran (see
// internal/table/lifecycle.go settle()), so presence as a Results key is
// exactly "reached showdown and was required to show".
func reachedShowdown(rec history.HandRecord, seatID int) bool {
	_, ok := rec.Results[seatID]
	return ok
}

// maskHoleCards returns a copy of rec whose Seats slice hides hole cards for
// every seat except the viewer's own and any seat that reached showdown. The
// stored record (and rec itself) is never mutated: history.Store keeps the
// full, unmasked data needed for the fairness reveal.
func maskHoleCards(rec history.HandRecord, viewerID string) history.HandRecord {
	masked := rec
	seats := make([]history.SeatInfo, len(rec.Seats))
	for i, seat := range rec.Seats {
		if seat.PlayerID == viewerID || reachedShowdown(rec, seat.SeatID) {
			seat.Hole = append([]string(nil), seat.Hole...)
		} else {
			seat.Hole = append([]string(nil), maskedHole...)
		}
		seats[i] = seat
	}
	masked.Seats = seats
	return masked
}

// isMaskedHole reports whether cards is the masked placeholder.
func isMaskedHole(cards []string) bool {
	return len(cards) == len(maskedHole) && cards[0] == maskedHole[0] && cards[1] == maskedHole[1]
}

// holeCardsSection renders the additive "*** HOLE CARDS ***" block for the
// text export: visible seats show their two cards, masked seats show
// "(mucked)". This is API-layer-only presentation; it does not change
// history.ExportText's own output.
func holeCardsSection(masked history.HandRecord) string {
	var b strings.Builder
	b.WriteString("*** HOLE CARDS ***\n")
	for _, seat := range masked.Seats {
		if isMaskedHole(seat.Hole) {
			fmt.Fprintf(&b, "Seat %d: (mucked)\n", seat.SeatID)
			continue
		}
		fmt.Fprintf(&b, "Seat %d: %s\n", seat.SeatID, strings.Join(seat.Hole, " "))
	}
	return b.String()
}

const (
	defaultHandsLimit = 20
	maxHandsLimit     = 100
)

// handSummary is one entry in the caller's recent-hands listing.
type handSummary struct {
	HandID    string `json:"handId"`
	TableID   string `json:"tableId"`
	StartedAt string `json:"startedAt"`
	PotWon    int64  `json:"potWon"`
}

// listMyHands handles GET /api/players/me/hands?limit=N: summaries of the
// caller's own recent hands, most-recent-first.
func (h *Handlers) listMyHands(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"))
	recs := h.store.ByPlayer(playerID, limit)

	out := make([]handSummary, 0, len(recs))
	for _, rec := range recs {
		out = append(out, handSummary{
			HandID:    rec.HandID,
			TableID:   rec.TableID,
			StartedAt: rec.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			PotWon:    potWonByPlayer(rec, playerID),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

// parseLimit reads the limit query param, defaulting to defaultHandsLimit and
// capping at maxHandsLimit. Non-positive or unparseable values fall back to
// the default.
func parseLimit(raw string) int {
	if raw == "" {
		return defaultHandsLimit
	}
	n := 0
	for _, c := range raw {
		if c < '0' || c > '9' {
			return defaultHandsLimit
		}
		n = n*10 + int(c-'0')
		if n > maxHandsLimit {
			return maxHandsLimit
		}
	}
	if n <= 0 {
		return defaultHandsLimit
	}
	if n > maxHandsLimit {
		return maxHandsLimit
	}
	return n
}

// potWonByPlayer sums the Awards a given playerID collected in rec, resolving
// PlayerID via the seat that received each award. Returns 0 if the player
// won nothing (including if they weren't dealt in, which callers never hit
// since ByPlayer already filters by seat membership).
func potWonByPlayer(rec history.HandRecord, playerID string) int64 {
	seatToPlayer := make(map[int]string, len(rec.Seats))
	for _, seat := range rec.Seats {
		seatToPlayer[seat.SeatID] = seat.PlayerID
	}
	var total int64
	for _, award := range rec.Awards {
		if seatToPlayer[award.SeatID] == playerID {
			total += int64(award.Amount)
		}
	}
	return total
}
