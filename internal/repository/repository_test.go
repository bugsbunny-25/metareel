package repository

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenReadOnly checks the read pool sees the writer's committed data and
// refuses writes, and that in-memory DSNs get no separate reader.
func TestOpenReadOnly(t *testing.T) {
	dsn := "file:" + filepath.Join(t.TempDir(), "r.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	w, err := Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if _, err := w.Exec(`CREATE TABLE t (v INTEGER); INSERT INTO t VALUES (7)`); err != nil {
		t.Fatal(err)
	}

	r, err := OpenReadOnly(dsn, 2)
	if err != nil || r == nil {
		t.Fatalf("OpenReadOnly = %v, %v", r, err)
	}
	defer r.Close()
	var v int
	if err := r.QueryRow(`SELECT v FROM t`).Scan(&v); err != nil || v != 7 {
		t.Fatalf("read = %d, %v", v, err)
	}
	if _, err := r.Exec(`INSERT INTO t VALUES (8)`); err == nil || !strings.Contains(err.Error(), "readonly") {
		t.Fatalf("write on reader: err = %v, want readonly", err)
	}
	// A write through the writer is visible to the reader once committed.
	if _, err := w.Exec(`UPDATE t SET v = 9`); err != nil {
		t.Fatal(err)
	}
	if err := r.QueryRow(`SELECT v FROM t`).Scan(&v); err != nil || v != 9 {
		t.Fatalf("read after write = %d, %v", v, err)
	}

	for _, d := range []string{"file::memory:?cache=shared", "file:x?mode=memory"} {
		if db, err := OpenReadOnly(d, 4); db != nil || err != nil {
			t.Fatalf("OpenReadOnly(%q) = %v, %v; want nil, nil", d, db, err)
		}
	}
	if db, err := OpenReadOnly(dsn, 0); db != nil || err != nil {
		t.Fatalf("OpenReadOnly(conns=0) = %v, %v; want nil, nil", db, err)
	}
}
