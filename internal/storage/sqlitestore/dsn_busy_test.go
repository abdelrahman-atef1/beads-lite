package sqlitestore

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// busy_timeout must come first so it covers the pragmas after it, and be long
// enough for a deep write queue (company-1bo5.10).
func TestDSNBusyTimeoutFirstAndLong(t *testing.T) {
	d := dsn(filepath.Join(t.TempDir(), "x.db"))
	q := d[strings.Index(d, "?")+1:]
	if !strings.HasPrefix(q, "_pragma=busy_timeout(30000)") {
		t.Fatalf("busy_timeout(30000) must be the first pragma: %s", q)
	}
}

// Many connections writing at once, each holding the lock for a while: the total
// wait exceeds the old 5 s per-waiter limit, yet nobody may fail.
func TestParallelWritersQueueWithoutBusy(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: queue wait > 5 s")
	}
	p := filepath.Join(t.TempDir(), "q.db")
	open := func() *sql.DB {
		db, err := openDB(dsn(p))
		if err != nil {
			t.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		return db
	}
	first := open()
	defer first.Close()
	if _, err := first.Exec(`CREATE TABLE t(i INTEGER)`); err != nil {
		t.Fatal(err)
	}
	const n = 30
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db := open()
			defer db.Close()
			tx, err := db.Begin()
			if err != nil {
				errs <- err
				return
			}
			time.Sleep(250 * time.Millisecond) // 30 x 250 ms = 7.5 s queue
			if _, err := tx.Exec(`INSERT INTO t VALUES (1)`); err != nil {
				errs <- err
			}
			if err := tx.Commit(); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("writer failed: %v", err)
	}
}
