// Package postgres provides PostgreSQL-backed implementations of the
// auth.Store, economy.Store, and history.Store interfaces, plus a small
// embedded migration runner.
//
// The existing Store interfaces (auth.Store, economy.Store, history.Store)
// are synchronous and take no context.Context. Every method in this package
// that talks to the database internally derives a context.Background() with
// a fixed opTimeout deadline for that single operation. This keeps call
// sites unchanged while still bounding query latency; callers that need
// cancellation/tracing propagation should use UpdateAtomic (which does take
// a ctx) or a future context-aware interface revision.
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// opTimeout bounds every individual database operation performed through the
// synchronous Store adapters in this package (see package doc comment).
const opTimeout = 5 * time.Second

// DB wraps a pgxpool.Pool and is the shared handle backing all Store
// implementations in this package.
type DB struct {
	*pgxpool.Pool
}

// Connect opens a pooled connection to dsn and verifies connectivity with a
// Ping. Callers must call Close when done.
func Connect(ctx context.Context, dsn string) (*DB, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}

	return &DB{Pool: pool}, nil
}

// Close releases all pooled connections.
func (db *DB) Close() {
	db.Pool.Close()
}

// opContext returns a background context bounded by opTimeout, used by the
// synchronous Store adapter methods (see package doc comment).
func opContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}
