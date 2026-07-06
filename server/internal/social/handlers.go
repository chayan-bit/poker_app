package social

import (
	"encoding/json"
	"net/http"
)

// AuthFunc resolves the caller's identity from the request, exactly like
// (*auth.Authenticator).FromRequest / lobby.AuthFunc / handsapi.AuthFunc. It
// is redefined locally (rather than imported) because auth's concrete
// Authenticator type is what callers hold; only the function shape is
// shared convention across packages in this codebase.
type AuthFunc func(*http.Request) (string, error)

// Handlers holds the friends/presence REST endpoints.
type Handlers struct {
	store       FriendStore
	presence    *PresenceTracker
	auth        AuthFunc
	resolveName func(playerID string) string
}

// New builds Handlers backed by store for the friend graph, presence for
// online/table state, auth for identity, and resolveName (optional - may be
// nil) to attach display names to responses.
func New(store FriendStore, presence *PresenceTracker, auth AuthFunc, resolveName func(string) string) *Handlers {
	return &Handlers{store: store, presence: presence, auth: auth, resolveName: resolveName}
}

// Register wires all friends/presence routes onto mux using Go 1.22+
// method+path patterns, matching handsapi.Register's convention.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/friends/request", h.request)
	mux.HandleFunc("POST /api/friends/accept", h.accept)
	mux.HandleFunc("POST /api/friends/decline", h.decline)
	mux.HandleFunc("POST /api/friends/remove", h.remove)
	mux.HandleFunc("GET /api/friends", h.list)
	mux.HandleFunc("GET /api/friends/pending", h.pending)
	mux.HandleFunc("GET /api/friends/{id}/table", h.friendTable)
}

// --- error envelope, matching internal/lobby and internal/handsapi's style ---

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
func (h *Handlers) requireAuth(w http.ResponseWriter, r *http.Request) (playerID string, ok bool) {
	playerID, err := h.auth(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid auth token")
		return "", false
	}
	return playerID, true
}

// --- request bodies ---

type playerIDRequest struct {
	PlayerID string `json:"playerId"`
}

func decodePlayerID(r *http.Request) (string, bool) {
	var req playerIDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PlayerID == "" {
		return "", false
	}
	return req.PlayerID, true
}

// friendErrorStatus maps a FriendStore error to an HTTP status + code.
func friendErrorStatus(err error) (int, string) {
	switch err {
	case ErrSelfRequest:
		return http.StatusBadRequest, "self_request"
	case ErrDuplicateRequest:
		return http.StatusConflict, "duplicate_request"
	case ErrAlreadyFriends:
		return http.StatusConflict, "already_friends"
	case ErrNoSuchRequest:
		return http.StatusNotFound, "no_such_request"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

// request handles POST /api/friends/request {"playerId": "..."}.
func (h *Handlers) request(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	target, ok := decodePlayerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "playerId is required")
		return
	}
	if err := h.store.Request(playerID, target); err != nil {
		status, code := friendErrorStatus(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// accept handles POST /api/friends/accept {"playerId": "..."}: playerId is
// the sender of the pending request the caller is accepting.
func (h *Handlers) accept(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	from, ok := decodePlayerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "playerId is required")
		return
	}
	if err := h.store.Accept(playerID, from); err != nil {
		status, code := friendErrorStatus(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// decline handles POST /api/friends/decline {"playerId": "..."}: playerId is
// the sender of the pending request the caller is declining.
func (h *Handlers) decline(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	from, ok := decodePlayerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "playerId is required")
		return
	}
	if err := h.store.Decline(playerID, from); err != nil {
		status, code := friendErrorStatus(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// remove handles POST /api/friends/remove {"playerId": "..."}.
func (h *Handlers) remove(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	friendID, ok := decodePlayerID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "playerId is required")
		return
	}
	if err := h.store.Remove(playerID, friendID); err != nil {
		status, code := friendErrorStatus(err)
		writeError(w, status, code, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// friendEntry is one entry in GET /api/friends.
type friendEntry struct {
	PlayerID string `json:"playerId"`
	Name     string `json:"name,omitempty"`
	Status   Status `json:"status"`
}

// list handles GET /api/friends.
func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	ids := h.store.FriendsOf(playerID)
	out := make([]friendEntry, 0, len(ids))
	for _, id := range ids {
		out = append(out, friendEntry{
			PlayerID: id,
			Name:     h.name(id),
			Status:   h.presence.Get(id),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// pendingEntry is one entry in GET /api/friends/pending.
type pendingEntry struct {
	PlayerID string `json:"playerId"`
	Name     string `json:"name,omitempty"`
}

// pending handles GET /api/friends/pending: incoming requests for the caller.
func (h *Handlers) pending(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	reqs := h.store.PendingFor(playerID)
	out := make([]pendingEntry, 0, len(reqs))
	for _, req := range reqs {
		out = append(out, pendingEntry{PlayerID: req.From, Name: h.name(req.From)})
	}
	writeJSON(w, http.StatusOK, out)
}

// friendTableResponse is the body of GET /api/friends/{id}/table.
type friendTableResponse struct {
	TableID string `json:"tableId"`
}

// friendTable handles GET /api/friends/{id}/table: 403 if id is not a
// confirmed friend of the caller, 404 if the friend is offline or not
// currently at a table, else the friend's current tableId.
func (h *Handlers) friendTable(w http.ResponseWriter, r *http.Request) {
	playerID, ok := h.requireAuth(w, r)
	if !ok {
		return
	}
	friendID := r.PathValue("id")

	isFriend := false
	for _, id := range h.store.FriendsOf(playerID) {
		if id == friendID {
			isFriend = true
			break
		}
	}
	if !isFriend {
		writeError(w, http.StatusForbidden, "not_friends", "that player is not a confirmed friend")
		return
	}

	st := h.presence.Get(friendID)
	if st.State != StateTable || st.TableID == "" {
		writeError(w, http.StatusNotFound, "not_at_table", "friend is offline or not currently at a table")
		return
	}
	writeJSON(w, http.StatusOK, friendTableResponse{TableID: st.TableID})
}

// name resolves a display name via resolveName, tolerating a nil hook.
func (h *Handlers) name(playerID string) string {
	if h.resolveName == nil {
		return ""
	}
	return h.resolveName(playerID)
}
