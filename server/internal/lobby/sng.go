package lobby

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/chayan-bit/poker_app/server/internal/economy"
	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/tourney"
)

// SNGManager is the subset of tourney.Manager the lobby depends on. Kept narrow
// (like Registry) so tourney signature drift is absorbed only at the wiring
// point; nil means the SNG endpoints are disabled.
type SNGManager interface {
	Create(cfg tourney.SNGConfig) (sngID, tableID string, err error)
	Register(sngID, playerID string) error
	List() []tourney.View
}

// WithSNG attaches a sit-and-go manager, enabling the /api/sng endpoints. It is
// additive: a Lobby built without it serves the existing cash endpoints only.
func (l *Lobby) WithSNG(m SNGManager) *Lobby {
	l.sng = m
	return l
}

// --- POST /api/sng ---

type createSNGRequest struct {
	Name  string       `json:"name"`
	Seats int          `json:"seats"`
	BuyIn engine.Chips `json:"buyIn"`
}

type createSNGResponse struct {
	SngID   string `json:"sngId"`
	TableID string `json:"tableId"`
}

// CreateSNG handles POST /api/sng: opens a sit-and-go and returns its id plus
// the (pre-allocated) table id it will run on.
func (l *Lobby) CreateSNG() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
			return
		}
		if l.sng == nil {
			writeError(w, http.StatusNotFound, "not_found", "tournaments are not enabled")
			return
		}
		if _, ok := l.requireAuth(w, r); !ok {
			return
		}

		var req createSNGRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed JSON body")
			return
		}
		if req.Seats < 2 || req.Seats > 9 {
			writeError(w, http.StatusBadRequest, "invalid_seats", "seats must be between 2 and 9")
			return
		}
		if req.BuyIn <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_buyin", "buyIn must be positive")
			return
		}

		sngID, tableID, err := l.sng.Create(tourney.DefaultConfig(req.Name, req.Seats, req.BuyIn))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, createSNGResponse{SngID: sngID, TableID: tableID})
	}
}

// --- POST /api/sng/register ---

type registerSNGRequest struct {
	SngID string `json:"sngId"`
}

// RegisterSNG handles POST /api/sng/register: registers the caller, collecting
// the buy-in. The final registration auto-starts the tournament.
func (l *Lobby) RegisterSNG() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
			return
		}
		if l.sng == nil {
			writeError(w, http.StatusNotFound, "not_found", "tournaments are not enabled")
			return
		}
		playerID, ok := l.requireAuth(w, r)
		if !ok {
			return
		}

		var req registerSNGRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SngID == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "sngId is required")
			return
		}

		switch err := l.sng.Register(req.SngID, playerID); {
		case err == nil:
			writeJSON(w, http.StatusOK, struct {
				Status string `json:"status"`
			}{"registered"})
		case errors.Is(err, tourney.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "no sit-and-go with that id")
		case errors.Is(err, tourney.ErrFull):
			writeError(w, http.StatusConflict, "sng_full", "registration is closed")
		case errors.Is(err, tourney.ErrAlreadyRegistered):
			writeError(w, http.StatusConflict, "already_registered", "already registered for this sit-and-go")
		case errors.Is(err, economy.ErrInsufficientFunds):
			writeError(w, http.StatusConflict, "insufficient_funds", "not enough chips for the buy-in")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "registration failed")
		}
	}
}

// --- GET /api/sng ---

// ListSNG handles GET /api/sng: the open sit-and-go listing.
func (l *Lobby) ListSNG() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
			return
		}
		if l.sng == nil {
			writeError(w, http.StatusNotFound, "not_found", "tournaments are not enabled")
			return
		}
		if _, ok := l.requireAuth(w, r); !ok {
			return
		}
		out := l.sng.List()
		if out == nil {
			out = []tourney.View{}
		}
		writeJSON(w, http.StatusOK, out)
	}
}
