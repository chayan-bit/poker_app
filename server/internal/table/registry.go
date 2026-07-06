package table

import (
	"sync"
	"time"
)

// Registry holds all live tables and indexes private rooms by join code.
// Safe for concurrent access from many connection goroutines.
//
// Deps is the dependency set handed to every table it creates. A zero-value
// Registry (from NewRegistry) leaves Deps zero; New fills production defaults,
// so tables still run. Wire real deps by setting Registry.Deps after
// construction (the lobby does this).
type Registry struct {
	mu     sync.RWMutex
	byID   map[string]*Table
	byCode map[string]*Table
	// byCreator counts live tables per creating playerID (Config.HostPlayerID),
	// used to bound goroutine growth from a single abusive creator. Tables with
	// no host (public quickseat tables) are not counted.
	byCreator     map[string]int
	maxPerCreator int // 0 == unlimited
	Deps          Deps
}

// NewRegistry constructs an empty registry with default (zero) deps.
func NewRegistry() *Registry {
	return &Registry{
		byID:      map[string]*Table{},
		byCode:    map[string]*Table{},
		byCreator: map[string]int{},
	}
}

// NewRegistryWithDeps constructs a registry whose tables receive deps.
func NewRegistryWithDeps(deps Deps) *Registry {
	return &Registry{
		byID:      map[string]*Table{},
		byCode:    map[string]*Table{},
		byCreator: map[string]int{},
		Deps:      deps,
	}
}

// SetMaxPerCreator bounds how many live tables one creator (Config.HostPlayerID)
// may own at once; 0 disables the cap. Enforced via CanCreateFor.
func (r *Registry) SetMaxPerCreator(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxPerCreator = n
}

// CanCreateFor reports whether playerID is under its live-table cap. An empty
// playerID (unowned public table) or a zero cap is always allowed. This is a
// soft, best-effort bound (a small check-then-create race is acceptable for a
// defense-in-depth limit).
func (r *Registry) CanCreateFor(playerID string) bool {
	if playerID == "" {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.maxPerCreator <= 0 || r.byCreator[playerID] < r.maxPerCreator
}

// Create registers and starts a new table using the registry's deps. It wires
// the table's idle-shutdown callback to Remove so a table that idles out also
// drops out of the registry's indexes.
func (r *Registry) Create(cfg Config) *Table {
	deps := r.Deps
	deps.OnShutdown = r.Remove
	t := New(cfg, deps)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[cfg.ID] = t
	if cfg.JoinCode != "" {
		r.byCode[cfg.JoinCode] = t
	}
	if cfg.HostPlayerID != "" {
		r.byCreator[cfg.HostPlayerID]++
	}
	return t
}

// CreateTourney registers and starts a tournament table. It reuses the
// registry's shared deps (ledger, history, clock) so a tournament draws on the
// same economy as cash tables, adds the per-table OnHandComplete callback that
// package tourney supplies for blind/elimination/payout sequencing, and wires
// idle shutdown to Remove. cfg.Tournament must be non-nil.
func (r *Registry) CreateTourney(cfg Config, onComplete OnHandComplete) *Table {
	deps := r.Deps
	deps.OnShutdown = r.Remove
	deps.OnHandComplete = onComplete
	t := New(cfg, deps)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[cfg.ID] = t
	if cfg.JoinCode != "" {
		r.byCode[cfg.JoinCode] = t
	}
	if cfg.HostPlayerID != "" {
		r.byCreator[cfg.HostPlayerID]++
	}
	return t
}

// Remove drops a table from the registry's indexes. Safe to call from the
// table's own goroutine (idle shutdown) - it takes only the registry lock.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.byID[id]; ok {
		delete(r.byID, id)
		if t.Cfg.JoinCode != "" {
			delete(r.byCode, t.Cfg.JoinCode)
		}
		if host := t.Cfg.HostPlayerID; host != "" {
			if n := r.byCreator[host]; n > 1 {
				r.byCreator[host] = n - 1
			} else {
				delete(r.byCreator, host)
			}
		}
	}
}

// DrainAll gracefully shuts down every live table: it drains each one (aborting
// in-flight hands with refunds and cashing out cash seats to the durable
// ledger), waiting up to deadline per table, then removes it from the indexes.
// Call this on process shutdown AFTER the HTTP server stops accepting new work
// and BEFORE closing the durable store, so all ledger writes flush first.
func (r *Registry) DrainAll(deadline time.Duration) {
	r.mu.RLock()
	tables := make([]*Table, 0, len(r.byID))
	for _, t := range r.byID {
		tables = append(tables, t)
	}
	r.mu.RUnlock()

	for _, t := range tables {
		t.Shutdown(deadline)
		r.Remove(t.Cfg.ID)
	}
}

// Get looks up a table by ID.
func (r *Registry) Get(id string) (*Table, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byID[id]
	return t, ok
}

// ByCode resolves a private room's 6-char join code (Design_suite 6.2).
func (r *Registry) ByCode(code string) (*Table, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byCode[code]
	return t, ok
}

// Public returns configs of joinable public tables for the lobby list.
func (r *Registry) Public() []Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Config
	for _, t := range r.byID {
		if t.Cfg.Visibility == Public {
			out = append(out, t.Cfg)
		}
	}
	return out
}
