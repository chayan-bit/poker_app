// Package lobby implements the REST surface for discovering and creating
// tables (Design_suite 6.2): the public table list, private room creation
// and join-by-code, and quickseat matchmaking. It never touches table
// internals directly; all access is through the small Registry interface
// below, which is intentionally narrow so upstream signature drift in
// internal/table only needs to be absorbed at the main.go wiring point.
package lobby

import (
	"encoding/json"
	"net/http"

	"github.com/chayan-bit/poker_app/server/internal/engine"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// Registry is the subset of table.Registry's API the lobby depends on.
type Registry interface {
	Create(table.Config) *table.Table
	ByCode(string) (*table.Table, bool)
	Public() []table.Config
}

// AuthFunc resolves the caller's identity from the request, exactly like
// (*auth.Authenticator).FromRequest.
type AuthFunc func(*http.Request) (string, error)

// quickseatBlinds are the only smallBlind values quickseat will match on
// (Design_suite 6.2 fixed stake tiers).
var quickseatBlinds = map[engine.Chips]bool{
	25:   true,
	50:   true,
	100:  true,
	500:  true,
	1000: true,
}

// Lobby holds handlers for table discovery/creation endpoints.
type Lobby struct {
	reg  Registry
	auth AuthFunc
}

// New builds a Lobby backed by reg for table state and auth for identity.
func New(reg Registry, auth AuthFunc) *Lobby {
	return &Lobby{reg: reg, auth: auth}
}

// apiError is the standard error envelope: {"error":{"code","message"}}.
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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// requireAuth resolves identity or writes a 401 and returns ok=false.
func (l *Lobby) requireAuth(w http.ResponseWriter, r *http.Request) (playerID string, ok bool) {
	playerID, err := l.auth(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid auth token")
		return "", false
	}
	return playerID, true
}
