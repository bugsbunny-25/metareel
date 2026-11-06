// Package repository owns all database access for the application.
//
// The typed query layer lives in ./sqlc and is generated from `db/queries/*.sql`
// using https://github.com/sqlc-dev/sqlc. Regenerate it after editing queries:
//
//	make sqlc
//
// This file only wires up the `*sql.DB` handle and migrations; higher-level
// code should depend on the generated `sqlc.Queries` (or an interface that
// wraps it) rather than this package directly.
package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Open opens a modernc SQLite database using the provided DSN. The DSN should
// be a full `file:` URL with `_pragma=...` query params – see `.env.example`.
func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite only supports a single writer at a time. Limiting to one
	// connection keeps writes serialized and avoids SQLITE_BUSY noise
	// under concurrent load; WAL mode (set via DSN pragma) handles reads.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}

// OpenReadOnly opens a pool of up to conns read-only connections to the same
// database as dsn (every connection runs `PRAGMA query_only`). In WAL mode
// these read alongside the single writer from Open, so API reads no longer
// queue behind job writes or each other. It returns (nil, nil) when conns < 1
// or dsn is an in-memory database (a second handle would see another DB).
func OpenReadOnly(dsn string, conns int) (*sql.DB, error) {
	if conns < 1 || strings.Contains(dsn, ":memory:") || strings.Contains(dsn, "mode=memory") {
		return nil, nil
	}
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	db, err := sql.Open("sqlite", dsn+sep+"_pragma=query_only(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite reader: %w", err)
	}
	db.SetMaxOpenConns(conns)
	db.SetMaxIdleConns(conns)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite reader: %w", err)
	}
	return db, nil
}

// Migrate runs all pending up migrations from `migrationsDir` using Goose.
// `migrationsDir` should be a filesystem path, e.g. `db/migrations`.
func Migrate(db *sql.DB, migrationsDir string) error {
	// Goose dialect is about migration version table + SQL helpers; it is
	// independent of the Go SQL driver name (we use modernc's "sqlite").
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("goose set dialect: %w", err)
	}
	if err := goose.Up(db, migrationsDir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
