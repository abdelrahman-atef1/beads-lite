package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/storage/backends"
	"github.com/steveyegge/beads/internal/storage/sqlitestore"
)

func sqliteGuardWorkspace(t *testing.T) string {
	t.Helper()
	pinLegacyUpgradeGuardEnv(t)
	if !backends.Registered(configfile.BackendSQLite) {
		sqlitestore.Register() // main() does this in production
	}
	dir := filepath.Join(t.TempDir(), ".beads")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	meta := `{"database":"beads.db","backend":"sqlite"}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// company-1bo5.9: a lite store admitted when quiet, never refused as historical
// while a parallel writer holds it; a real historical store is still refused.
func TestGuardSQLiteLiteStoreNotRefusedUnderLock(t *testing.T) {
	dir := sqliteGuardWorkspace(t)
	defer sqlitestore.SetProbeTimingForTest(3, 20)()
	p := filepath.Join(dir, "beads.db")
	s, err := sqlitestore.Open(context.Background(), p, sqlitestore.Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if err := guardLegacyUpgradeWorkspace(dir); err != nil {
		t.Fatalf("quiet lite store refused: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(p)+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(1000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA locking_mode=EXCLUSIVE", "BEGIN IMMEDIATE"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	err = guardLegacyUpgradeWorkspace(dir)
	if err == nil {
		t.Fatal("locked store: expected a retryable error")
	}
	if isLegacyUpgradeRefusal(err) {
		t.Fatalf("locked lite store must not be a legacy refusal: %v", err)
	}
	_, _ = db.Exec("ROLLBACK")
}

func TestGuardSQLiteHistoricalStoreStillRefused(t *testing.T) {
	dir := sqliteGuardWorkspace(t)
	p := filepath.Join(dir, "beads.db")
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(p))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE issues (id TEXT)"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if err := guardLegacyUpgradeWorkspace(dir); !isLegacyUpgradeRefusal(err) {
		t.Fatalf("historical sqlite store: %v, want refusal", err)
	}
}
