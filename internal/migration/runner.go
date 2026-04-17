// Package migration provides a lightweight, dependency-free database migration
// runner. It reads versioned SQL files from an [io/fs.FS] (typically embedded
// via [embed.FS]), tracks which migrations have already been applied in a
// schema_migrations table, and applies or rolls back migrations as requested.
//
// Migration files must follow the naming convention:
//
//	<version>_<description>.up.sql   — forward migration
//	<version>_<description>.down.sql — rollback migration
//
// where <version> is a zero-padded integer (e.g. "0001"). Files are applied in
// ascending version order. Each migration is executed inside a single database
// transaction; if the SQL fails the transaction is rolled back and an error is
// returned.
package migration

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

const createSchemaTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT        NOT NULL PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`

// Runner applies and rolls back SQL migrations stored in an [io/fs.FS].
type Runner struct {
	db  *sql.DB
	fsys fs.FS
}

// New creates a Runner that reads migrations from fsys and applies them to db.
// The fsys is expected to contain files matching the pattern
// "migrations/<version>_<description>.<direction>.sql".
func New(db *sql.DB, fsys fs.FS) *Runner {
	return &Runner{db: db, fsys: fsys}
}

// Up applies all unapplied forward (.up.sql) migrations in ascending version
// order. Migrations that have already been recorded in schema_migrations are
// skipped. Each migration runs inside its own transaction.
func (r *Runner) Up(ctx context.Context) (int, error) {
	if err := r.ensureTable(ctx); err != nil {
		return 0, fmt.Errorf("migration: ensure schema_migrations table: %w", err)
	}

	files, err := r.listFiles("up")
	if err != nil {
		return 0, fmt.Errorf("migration: list up-migration files: %w", err)
	}

	applied, err := r.appliedVersions(ctx)
	if err != nil {
		return 0, fmt.Errorf("migration: load applied versions: %w", err)
	}

	count := 0
	for _, f := range files {
		version := VersionFromPath(f)
		if applied[version] {
			continue
		}
		if err := r.applyFile(ctx, f, version); err != nil {
			return count, fmt.Errorf("migration: apply %s: %w", f, err)
		}
		count++
	}

	return count, nil
}

// Down rolls back the single most-recently-applied migration by executing the
// corresponding .down.sql file and removing the version from schema_migrations.
// If no migrations have been applied, Down returns nil with a count of zero.
func (r *Runner) Down(ctx context.Context) (int, error) {
	if err := r.ensureTable(ctx); err != nil {
		return 0, fmt.Errorf("migration: ensure schema_migrations table: %w", err)
	}

	latest, err := r.latestVersion(ctx)
	if err != nil {
		return 0, fmt.Errorf("migration: query latest version: %w", err)
	}
	if latest == "" {
		return 0, nil
	}

	// Locate the matching .down.sql file.
	files, err := r.listFiles("down")
	if err != nil {
		return 0, fmt.Errorf("migration: list down-migration files: %w", err)
	}

	var target string
	for _, f := range files {
		if VersionFromPath(f) == latest {
			target = f
			break
		}
	}
	if target == "" {
		return 0, fmt.Errorf("migration: no .down.sql file found for version %s", latest)
	}

	if err := r.rollbackFile(ctx, target, latest); err != nil {
		return 0, fmt.Errorf("migration: rollback %s: %w", target, err)
	}

	return 1, nil
}

// Status returns every version recorded in schema_migrations together with the
// timestamp at which it was applied. Versions are returned in ascending order.
func (r *Runner) Status(ctx context.Context) ([]AppliedMigration, error) {
	if err := r.ensureTable(ctx); err != nil {
		return nil, fmt.Errorf("migration: ensure schema_migrations table: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT version, applied_at FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		return nil, fmt.Errorf("migration: query status: %w", err)
	}
	defer rows.Close()

	var result []AppliedMigration
	for rows.Next() {
		var m AppliedMigration
		if err := rows.Scan(&m.Version, &m.AppliedAt); err != nil {
			return nil, fmt.Errorf("migration: scan row: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("migration: iterate rows: %w", err)
	}

	return result, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Unexported helpers
// ──────────────────────────────────────────────────────────────────────────────

func (r *Runner) ensureTable(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, createSchemaTable)
	return err
}

// listFiles returns all migration file paths for the given direction ("up" or
// "down") in ascending version order.
func (r *Runner) listFiles(direction string) ([]string, error) {
	suffix := "." + direction + ".sql"

	var paths []string
	err := fs.WalkDir(r.fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, suffix) {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)
	return paths, nil
}

func (r *Runner) appliedVersions(ctx context.Context) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

func (r *Runner) latestVersion(ctx context.Context) (string, error) {
	var version sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`,
	).Scan(&version)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return version.String, nil
}

func (r *Runner) applyFile(ctx context.Context, path, version string) error {
	content, err := r.readFile(path)
	if err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, content); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`, version,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Runner) rollbackFile(ctx context.Context, path, version string) error {
	content, err := r.readFile(path)
	if err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx, content); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM schema_migrations WHERE version = $1`, version,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Runner) readFile(path string) (string, error) {
	f, err := r.fsys.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}

// VersionFromPath extracts the version prefix (everything up to the first dot
// in the base name) from a migration file path.
//
// Examples:
//
//	"migrations/0001_initial_schema.up.sql" → "0001_initial_schema"
//	"0002_add_users.down.sql"               → "0002_add_users"
func VersionFromPath(path string) string {
	base := filepath.Base(path)
	// Strip the first suffix (".sql"), then the second (".up" / ".down").
	withoutSQL := strings.TrimSuffix(base, ".sql")
	withoutDir := strings.TrimSuffix(withoutSQL, filepath.Ext(withoutSQL))
	return withoutDir
}
