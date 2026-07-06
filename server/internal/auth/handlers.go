package auth

import (
	"encoding/json"
	"net/http"
)

// guestResponse is returned by POST /api/auth/guest.
type guestResponse struct {
	Token    string `json:"token"`
	PlayerID string `json:"playerId"`
}

// upgradeRequest is the body of POST /api/auth/upgrade.
type upgradeRequest struct {
	Email string `json:"email"`
}

// upgradeResponse is returned by POST /api/auth/upgrade.
type upgradeResponse struct {
	Token    string `json:"token"`
	PlayerID string `json:"playerId"`
	Email    string `json:"email"`
}

// GuestHandler creates a new guest account and returns a signed guest token.
func (a *Authenticator) GuestHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		acc := a.store.CreateGuest()
		tok, err := a.tokens.Issue(acc.PlayerID, true, GuestTokenTTL)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, guestResponse{Token: tok, PlayerID: acc.PlayerID})
	}
}

// UpgradeHandler upgrades the caller's guest identity to a full account tied
// to an email, preserving the PlayerID (and thus chips/history).
func (a *Authenticator) UpgradeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		playerID, err := a.FromRequest(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req upgradeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		acc, err := a.store.UpgradeToAccount(playerID, req.Email)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}

		tok, err := a.tokens.Issue(acc.PlayerID, false, AccountTokenTTL)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, upgradeResponse{Token: tok, PlayerID: acc.PlayerID, Email: acc.Email})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
