package social

import "sync"

// Presence state values.
const (
	StateOffline = "offline"
	StateLobby   = "lobby"
	StateTable   = "table"
)

// Status is a player's current online/location state.
type Status struct {
	State   string `json:"state"`   // "offline" | "lobby" | "table"
	TableID string `json:"tableId"` // non-empty only when State == StateTable
}

// PresenceTracker is an in-memory, thread-safe presence store.
//
// This is a simple mutex-guarded map today. A Redis-backed implementation
// (e.g. a hash of playerID -> {state,tableId} with TTL-based expiry for
// offline detection) could satisfy the same method set later for horizontal
// scaling across multiple pokerd instances; nothing here assumes a single
// process beyond the in-memory map itself.
type PresenceTracker struct {
	mu     sync.Mutex
	byUser map[string]Status
}

// NewPresenceTracker builds an empty tracker; unknown players default to
// StateOffline via Get.
func NewPresenceTracker() *PresenceTracker {
	return &PresenceTracker{byUser: make(map[string]Status)}
}

// SetLobby marks playerID as present in the lobby (not seated at any table).
func (p *PresenceTracker) SetLobby(playerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byUser[playerID] = Status{State: StateLobby}
}

// SetTable marks playerID as present at tableID.
func (p *PresenceTracker) SetTable(playerID, tableID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byUser[playerID] = Status{State: StateTable, TableID: tableID}
}

// SetOffline marks playerID as disconnected/offline.
func (p *PresenceTracker) SetOffline(playerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.byUser[playerID] = Status{State: StateOffline}
}

// Get returns playerID's current status, defaulting to {State: StateOffline}
// for a player never seen before.
func (p *PresenceTracker) Get(playerID string) Status {
	p.mu.Lock()
	defer p.mu.Unlock()
	if st, ok := p.byUser[playerID]; ok {
		return st
	}
	return Status{State: StateOffline}
}

// Presence is the process-wide presence singleton. Other packages that
// cannot cleanly import internal/social types elsewhere (or that just want a
// zero-ceremony call site) can reach it directly as social.Presence.
//
// Wiring instructions for the orchestrator (not done in this change per
// scope guard - see final report):
//   - In the WS gateway's connection-accepted / join_table path: call
//     social.Presence.SetTable(playerID, tableID) once a player is
//     subscribed to a table, and social.Presence.SetLobby(playerID) when
//     they navigate back to the lobby without a table.
//   - On WS disconnect (after the table's own disconnect-grace handling
//     decides the player is truly gone, or immediately if you want presence
//     to reflect socket state rather than seat state): call
//     social.Presence.SetOffline(playerID).
//   - TouchFunc below is provided so a caller can store a typed reference to
//     one of these methods without importing social's Status type at the
//     call site, e.g.:
//       var onTableJoin social.TouchFunc = social.Presence.SetTable
var Presence = NewPresenceTracker()

// TouchFunc is the shape of PresenceTracker.SetTable (and, ignoring the
// second argument, can be adapted for SetLobby/SetOffline by callers that
// want a uniform function value to pass around, e.g. from the WS gateway's
// connection lifecycle without depending on *PresenceTracker directly).
type TouchFunc func(playerID, tableID string)
