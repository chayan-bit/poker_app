package table

import "sync"

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
	Deps   Deps
}

// NewRegistry constructs an empty registry with default (zero) deps.
func NewRegistry() *Registry {
	return &Registry{byID: map[string]*Table{}, byCode: map[string]*Table{}}
}

// NewRegistryWithDeps constructs a registry whose tables receive deps.
func NewRegistryWithDeps(deps Deps) *Registry {
	return &Registry{byID: map[string]*Table{}, byCode: map[string]*Table{}, Deps: deps}
}

// Create registers and starts a new table using the registry's deps.
func (r *Registry) Create(cfg Config) *Table {
	t := New(cfg, r.Deps)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[cfg.ID] = t
	if cfg.JoinCode != "" {
		r.byCode[cfg.JoinCode] = t
	}
	return t
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
