package table

import "sync"

// Registry holds all live tables and indexes private rooms by join code.
// Safe for concurrent access from many connection goroutines.
type Registry struct {
	mu     sync.RWMutex
	byID   map[string]*Table
	byCode map[string]*Table
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{byID: map[string]*Table{}, byCode: map[string]*Table{}}
}

// Create registers and starts a new table.
func (r *Registry) Create(cfg Config) *Table {
	t := New(cfg)
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
