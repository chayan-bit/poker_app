package lobby

import (
	"encoding/json"
	"net/http"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// --- GET /api/tables ---

// publicTable is one entry in the public table listing.
type publicTable struct {
	TableID    string       `json:"tableId"`
	SmallBlind engine.Chips `json:"smallBlind"`
	BigBlind   engine.Chips `json:"bigBlind"`
	MaxSeats   int          `json:"maxSeats"`
	// TODO(lobby): expose current seat occupancy once table.Registry/Table
	// surfaces a seat-count accessor; the current API only exposes Config.
}

// ListTables handles GET /api/tables: the public lobby list.
func (l *Lobby) ListTables() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
			return
		}
		if _, ok := l.requireAuth(w, r); !ok {
			return
		}

		cfgs := l.reg.Public()
		out := make([]publicTable, 0, len(cfgs))
		for _, cfg := range cfgs {
			out = append(out, publicTable{
				TableID:    cfg.ID,
				SmallBlind: cfg.SmallBlind,
				BigBlind:   cfg.BigBlind,
				MaxSeats:   cfg.MaxSeats,
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// --- POST /api/rooms ---

type createRoomRequest struct {
	SmallBlind engine.Chips `json:"smallBlind"`
	BigBlind   engine.Chips `json:"bigBlind"`
	MaxSeats   int          `json:"maxSeats"`
	Visibility string       `json:"visibility"`
}

type createRoomResponse struct {
	TableID  string `json:"tableId"`
	JoinCode string `json:"joinCode"`
	JoinURL  string `json:"joinUrl"`
}

// CreateRoom handles POST /api/rooms: creates a private table and returns its
// join code (Design_suite 6.2).
func (l *Lobby) CreateRoom() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
			return
		}
		if _, ok := l.requireAuth(w, r); !ok {
			return
		}

		var req createRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed JSON body")
			return
		}
		if req.Visibility != "private" {
			writeError(w, http.StatusBadRequest, "invalid_visibility", `visibility must be "private"`)
			return
		}
		if err := validateBlinds(req.SmallBlind, req.BigBlind); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_blinds", err.Error())
			return
		}
		if req.MaxSeats < 2 || req.MaxSeats > 10 {
			writeError(w, http.StatusBadRequest, "invalid_max_seats", "maxSeats must be between 2 and 10")
			return
		}

		id, err := newTableID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate table id")
			return
		}
		code, err := newJoinCode()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate join code")
			return
		}

		t := l.reg.Create(table.Config{
			ID:         id,
			Visibility: table.Private,
			MaxSeats:   req.MaxSeats,
			SmallBlind: req.SmallBlind,
			BigBlind:   req.BigBlind,
			JoinCode:   code,
		})

		writeJSON(w, http.StatusOK, createRoomResponse{
			TableID:  t.Cfg.ID,
			JoinCode: code,
			JoinURL:  "/t/" + code,
		})
	}
}

// validateBlinds enforces smallBlind/bigBlind > 0 and bigBlind > smallBlind.
func validateBlinds(small, big engine.Chips) error {
	if small <= 0 || big <= 0 {
		return errBlindsPositive
	}
	if big <= small {
		return errBigBlindGreater
	}
	return nil
}

// --- POST /api/rooms/join ---

type joinRoomRequest struct {
	Code string `json:"code"`
}

type joinRoomResponse struct {
	TableID string `json:"tableId"`
}

// JoinRoom handles POST /api/rooms/join: resolves a private room's join code.
func (l *Lobby) JoinRoom() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
			return
		}
		if _, ok := l.requireAuth(w, r); !ok {
			return
		}

		var req joinRoomRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "code is required")
			return
		}

		t, ok := l.reg.ByCode(req.Code)
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "no room with that code")
			return
		}
		writeJSON(w, http.StatusOK, joinRoomResponse{TableID: t.Cfg.ID})
	}
}

// --- POST /api/quickseat ---

type quickseatRequest struct {
	SmallBlind engine.Chips `json:"smallBlind"`
}

type quickseatResponse struct {
	TableID string `json:"tableId"`
}

// quickseatMaxSeats is the fixed seat count for auto-created quickseat tables.
const quickseatMaxSeats = 6

// Quickseat handles POST /api/quickseat: joins an existing public table at the
// requested stake, or creates one if none exists yet (Design_suite 6.2).
func (l *Lobby) Quickseat() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
			return
		}
		if _, ok := l.requireAuth(w, r); !ok {
			return
		}

		var req quickseatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed JSON body")
			return
		}
		if !quickseatBlinds[req.SmallBlind] {
			writeError(w, http.StatusBadRequest, "invalid_stake", "smallBlind is not an allowed quickseat stake")
			return
		}

		for _, cfg := range l.reg.Public() {
			if cfg.SmallBlind == req.SmallBlind {
				writeJSON(w, http.StatusOK, quickseatResponse{TableID: cfg.ID})
				return
			}
		}

		id, err := newTableID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to generate table id")
			return
		}
		t := l.reg.Create(table.Config{
			ID:         id,
			Visibility: table.Public,
			MaxSeats:   quickseatMaxSeats,
			SmallBlind: req.SmallBlind,
			BigBlind:   2 * req.SmallBlind,
		})
		writeJSON(w, http.StatusOK, quickseatResponse{TableID: t.Cfg.ID})
	}
}
