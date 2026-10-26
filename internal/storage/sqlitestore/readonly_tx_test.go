package sqlitestore

import (
	"context"
	"path/filepath"
	"testing"
)

// lite: a read-only handle must be able to begin a transaction (reads such as
// list/show/ready run inside one). With _txlock=immediate this failed with
// "attempt to write a readonly database".
func TestReadOnlyStoreCanBeginTx(t *testing.T) {
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "beads.db")
	w, err := Open(ctx, p, Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	r, err := Open(ctx, p, Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	tx, err := r.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("read-only BeginTx: %v", err)
	}
	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM issues").Scan(&n); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback()
}
