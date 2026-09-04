package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const migrationLedger = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version bigint PRIMARY KEY,
    name text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
)`

var migrationFilePattern = regexp.MustCompile(`^(\d{6})_([a-z0-9_]+)\.up\.sql$`)

type Migration struct {
	Version int64
	Name    string
	SQL     string
}

// LoadMigrations reads only forward migrations with the strict project
// naming convention. Down migrations are never applied automatically.
func LoadMigrations(directory string) ([]Migration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}
	migrations := make([]Migration, 0, len(entries))
	seen := make(map[int64]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationFilePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if previous, exists := seen[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d: %s and %s", version, previous, entry.Name())
		}
		data, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		name := match[2]
		seen[version] = entry.Name()
		migrations = append(migrations, Migration{Version: version, Name: name, SQL: string(data)})
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	if len(migrations) == 0 {
		return nil, errors.New("no forward migrations found")
	}
	return migrations, nil
}

// ApplyMigrations applies each migration atomically and records it in a
// durable ledger. A transaction advisory lock prevents two deploy jobs from
// racing to apply the same schema change.
func ApplyMigrations(ctx context.Context, conn *pgx.Conn, migrations []Migration) error {
	if conn == nil {
		return errors.New("database connection is required")
	}
	if len(migrations) == 0 {
		return errors.New("migrations are required")
	}
	if _, err := conn.Exec(ctx, migrationLedger); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	for _, migration := range migrations {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %06d: %w", migration.Version, err)
		}
		applied, applyErr := applyOne(ctx, tx, migration)
		if applyErr != nil {
			_ = tx.Rollback(ctx)
			return applyErr
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %06d: %w", migration.Version, err)
		}
		if applied {
			continue
		}
	}
	return nil
}

func applyOne(ctx context.Context, tx pgx.Tx, migration Migration) (bool, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(847362190441)`); err != nil {
		return false, fmt.Errorf("lock migration ledger: %w", err)
	}
	var appliedName string
	err := tx.QueryRow(ctx, `SELECT name FROM schema_migrations WHERE version = $1`, migration.Version).Scan(&appliedName)
	switch {
	case err == nil:
		if appliedName != migration.Name {
			return false, fmt.Errorf("migration %06d name mismatch: database has %q, binary has %q", migration.Version, appliedName, migration.Name)
		}
		return true, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return false, fmt.Errorf("read migration %06d ledger: %w", migration.Version, err)
	}
	if _, err := tx.Exec(ctx, migration.SQL); err != nil {
		return false, fmt.Errorf("apply migration %06d_%s: %w", migration.Version, migration.Name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version, name) VALUES ($1, $2)`, migration.Version, migration.Name); err != nil {
		return false, fmt.Errorf("record migration %06d: %w", migration.Version, err)
	}
	return false, nil
}
