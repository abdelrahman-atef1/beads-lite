package sqlitestore

import (
	"context"
	"os"
	"testing"
	"time"
)

// holdExclusive opens path with an exclusive WAL locking mode and reads once,
// which makes every other connection get SQLITE_BUSY until release is called.
func holdExclusive(t *testing.T, p string) (release func()) {
	t.Helper()
	db, err := openDB(dsn(p))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA locking_mode=EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	return func() { _, _ = db.Exec("ROLLBACK"); _ = db.Close() }
}

func shortProbe(t *testing.T) {
	t.Helper()
	a, b := probeAttempts, probeBusyMS
	probeAttempts, probeBusyMS = 3, 20
	t.Cleanup(func() { probeAttempts, probeBusyMS = a, b })
}

func TestProbeLiteStoreDefiniteAnswers(t *testing.T) {
	ctx := context.Background()
	lite := dbPath(t)
	s, err := Open(ctx, lite, Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	if ok, err := ProbeLiteStore(lite); err != nil || !ok {
		t.Fatalf("lite store: ok=%v err=%v", ok, err)
	}

	old := dbPath(t) // a historical store: valid SQLite, no lite table
	db, _ := openDB(dsn(old))
	_, _ = db.Exec("CREATE TABLE issues (id TEXT)")
	_ = db.Close()
	if ok, err := ProbeLiteStore(old); err != nil || ok {
		t.Fatalf("old sqlite store must be a definite no: ok=%v err=%v", ok, err)
	}

	junk := dbPath(t)
	if err := os.WriteFile(junk, []byte("SQLite format 3\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ok, err := ProbeLiteStore(junk); err != nil || ok {
		t.Fatalf("unreadable-as-sqlite file must be a definite no: ok=%v err=%v", ok, err)
	}
}

// company-1bo5.9: a lock held by a parallel writer must not read as "not ours".
func TestProbeLiteStoreBusyIsAnErrorNotNo(t *testing.T) {
	ctx := context.Background()
	p := dbPath(t)
	s, err := Open(ctx, p, Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	shortProbe(t)
	release := holdExclusive(t, p)
	ok, err := ProbeLiteStore(p)
	if err == nil {
		t.Fatalf("held lock: want error, got ok=%v", ok)
	}
	if IsLiteStore(p) {
		t.Fatal("IsLiteStore must stay false on error")
	}
	release()
	if ok, err := ProbeLiteStore(p); err != nil || !ok {
		t.Fatalf("after release: ok=%v err=%v", ok, err)
	}
}

func TestProbeLiteStoreWaitsOutShortLock(t *testing.T) {
	ctx := context.Background()
	p := dbPath(t)
	s, err := Open(ctx, p, Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	probeAttempts, probeBusyMS = 8, 50
	t.Cleanup(func() { probeAttempts, probeBusyMS = 8, 5000 })
	release := holdExclusive(t, p)
	go func() { time.Sleep(300 * time.Millisecond); release() }()
	if ok, err := ProbeLiteStore(p); err != nil || !ok {
		t.Fatalf("short lock: ok=%v err=%v", ok, err)
	}
}
