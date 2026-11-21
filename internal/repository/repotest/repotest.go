// Package repotest gives tests a migrated SQLite database of their own.
//
// Running every migration is most of a database test's setup time (about
// 1.5 s per database under -race, against well under a millisecond to copy
// the result). So each test binary migrates one template database the first
// time it is asked for, and every test gets a byte copy of that file: the
// schema and seed rows a fresh migration would give, in a file of its own.
// Tests of the migrations themselves keep running repository.Migrate.
package repotest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/bugsbunny-25/metareel/internal/repository"
)

var (
	mu        sync.Mutex
	templates = map[string][]byte{} // absolute migrations dir -> migrated database file
)

// Open writes a migrated database to path, opens it with foreign keys on, and
// closes it when the test ends. migrationsDir is as for repository.Migrate
// (relative to the test's package directory).
func Open(t testing.TB, migrationsDir, path string) *sql.DB {
	t.Helper()
	image, err := template(migrationsDir)
	if err != nil {
		t.Fatalf("migrate the template database: %v", err)
	}
	if err := os.WriteFile(path, image, 0o600); err != nil {
		t.Fatalf("write test database: %v", err)
	}
	db, err := repository.Open(dsn(path))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func dsn(path string) string {
	return "file:" + path + "?_pragma=foreign_keys(1)"
}

// template returns the bytes of a database migrated from dir, migrating it on
// the first call. A failed migration is not cached, so every caller sees it.
func template(dir string) ([]byte, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	mu.Lock()
	defer mu.Unlock()
	if image, ok := templates[abs]; ok {
		return image, nil
	}

	tmp, err := os.MkdirTemp("", "metareel-template-db-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	path := filepath.Join(tmp, "template.db")
	db, err := repository.Open(dsn(path))
	if err != nil {
		return nil, err
	}
	if err := repository.Migrate(db, abs); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Closing the only connection leaves everything in the one file (the
	// default rollback journal is gone once a transaction ends).
	if err := db.Close(); err != nil {
		return nil, fmt.Errorf("close template database: %w", err)
	}
	image, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	templates[abs] = image
	return image, nil
}
