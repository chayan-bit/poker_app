package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration is one parsed, versioned SQL migration file.
type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations reads and parses every embedded migration file, returning
// them sorted by ascending version. File names must look like
// "0001_description.sql"; the leading numeric prefix is the version.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("postgres: read migrations dir: %w", err)
	}

	migs := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		version, err := parseVersion(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("postgres: migration %s: %w", entry.Name(), err)
		}

		contents, err := fs.ReadFile(migrationsFS, "migrations/"+entry.Name())
		if err != nil {
			return nil, fmt.Errorf("postgres: read migration %s: %w", entry.Name(), err)
		}

		migs = append(migs, migration{version: version, name: entry.Name(), sql: string(contents)})
	}

	sort.Slice(migs, func(i, j int) bool { return migs[i].version < migs[j].version })

	for i := 1; i < len(migs); i++ {
		if migs[i].version == migs[i-1].version {
			return nil, fmt.Errorf("postgres: duplicate migration version %d (%s, %s)",
				migs[i].version, migs[i-1].name, migs[i].name)
		}
	}

	return migs, nil
}

// parseVersion extracts the leading "NNNN" numeric prefix from a migration
// file name such as "0001_init.sql".
func parseVersion(fileName string) (int, error) {
	prefix, _, ok := strings.Cut(fileName, "_")
	if !ok {
		return 0, fmt.Errorf("expected NNNN_name.sql, got %q", fileName)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("expected numeric version prefix, got %q: %w", prefix, err)
	}
	return version, nil
}

// Migrate applies every embedded migration not yet recorded in the
// schema_migrations table, each inside its own transaction, in ascending
// version order.
func (db *DB) Migrate(ctx context.Context) error {
	migs, err := loadMigrations()
	if err != nil {
		return err
	}

	if _, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     integer PRIMARY KEY,
			name        text NOT NULL,
			applied_at  timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("postgres: create schema_migrations: %w", err)
	}

	for _, m := range migs {
		var applied bool
		if err := db.Pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, m.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("postgres: check migration %d: %w", m.version, err)
		}
		if applied {
			continue
		}

		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("postgres: begin migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec(ctx, m.sql); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("postgres: apply migration %d (%s): %w", m.version, m.name, err)
		}

		if _, err := tx.Exec(ctx,
			`INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.version, m.name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("postgres: record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("postgres: commit migration %d: %w", m.version, err)
		}
	}

	return nil
}
