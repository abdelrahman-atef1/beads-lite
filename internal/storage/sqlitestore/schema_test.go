package sqlitestore

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dbPath(t *testing.T) string { return filepath.Join(t.TempDir(), "beads.db") }

func TestCreateOpenAndReopen(t *testing.T) {
	ctx := context.Background()
	p := dbPath(t)
	s, err := Open(ctx, p, Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	var v int
	if err := s.DB().QueryRow("PRAGMA user_version").Scan(&v); err != nil || v != 1 {
		t.Fatalf("user_version=%d err=%v", v, err)
	}
	var fk, n int
	_ = s.DB().QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if fk != 1 {
		t.Fatal("foreign_keys not on")
	}
	var mode string
	_ = s.DB().QueryRow("PRAGMA journal_mode").Scan(&mode)
	if mode != "wal" {
		t.Fatalf("journal_mode=%s", mode)
	}
	_ = s.DB().QueryRow("SELECT count(*) FROM schema_migrations").Scan(&n)
	if n != 66 {
		t.Fatalf("schema_migrations=%d", n)
	}
	_ = s.DB().QueryRow("SELECT count(*) FROM ignored_schema_migrations").Scan(&n)
	if n != 26 {
		t.Fatalf("ignored_schema_migrations=%d", n)
	}
	_ = s.DB().QueryRow("SELECT count(*) FROM config").Scan(&n)
	if n != 9 {
		t.Fatalf("config seeds=%d", n)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, p, Options{}) // reopen without Create
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
}

func TestMissingFileRefusedWithoutCreate(t *testing.T) {
	p := dbPath(t)
	if _, err := Open(context.Background(), p, Options{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("file was created")
	}
}

func TestNewerSchemaRefused(t *testing.T) {
	ctx := context.Background()
	p := dbPath(t)
	s, _ := Open(ctx, p, Options{Create: true})
	if _, err := s.DB().Exec("PRAGMA user_version = 2"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, err := Open(ctx, p, Options{})
	if err == nil || !strings.Contains(err.Error(), "newer beads-lite (schema 2)") {
		t.Fatalf("err=%v", err)
	}
}

func TestUnknownDatabaseRefused(t *testing.T) {
	ctx := context.Background()
	p := dbPath(t)
	db, _ := openDB(dsn(p))
	_, _ = db.Exec("CREATE TABLE foreign_thing (x)")
	_ = db.Close()
	_, err := Open(ctx, p, Options{Create: true})
	if err == nil || !strings.Contains(err.Error(), "unrecognised") {
		t.Fatalf("err=%v", err)
	}
}

func TestConcurrentCreateSecondOpenerAccepts(t *testing.T) {
	ctx := context.Background()
	p := dbPath(t)
	a, err := Open(ctx, p, Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(ctx, p, Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = a.Close()
	_ = b.Close()
}

func TestRunInTxHooksAndRollback(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, dbPath(t), Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	hookRan := 0
	s.RegisterBeforeCommit(func(ctx context.Context, tx *sql.Tx) error { hookRan++; return nil })
	ins := func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO issues (id,title,description,design,acceptance_criteria,notes) VALUES ('t-1','x','','','','')")
		return err
	}
	if err := s.RunInTx(ctx, ins); err != nil || hookRan != 1 {
		t.Fatalf("err=%v hook=%d", err, hookRan)
	}
	var touched int
	_ = s.DB().QueryRow("SELECT count(*) FROM lite_touched").Scan(&touched)
	if touched != 1 {
		t.Fatalf("trigger did not record touched row: %d", touched)
	}
	// A failing hook rolls the whole write back.
	s.RegisterBeforeCommit(func(ctx context.Context, tx *sql.Tx) error { return os.ErrInvalid })
	err = s.RunInTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO issues (id,title,description,design,acceptance_criteria,notes) VALUES ('t-2','y','','','','')")
		return err
	})
	if err == nil {
		t.Fatal("expected hook error")
	}
	var n int
	_ = s.DB().QueryRow("SELECT count(*) FROM issues").Scan(&n)
	if n != 1 {
		t.Fatalf("rollback failed, issues=%d", n)
	}
}

func TestResolvePathDefault(t *testing.T) {
	dir := t.TempDir()
	p, err := ResolvePath(dir)
	if err != nil || p != filepath.Join(dir, "beads.db") {
		t.Fatalf("p=%q err=%v", p, err)
	}
}
