package sqlitestore

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

// IsLiteStore reports whether the SQLite file at path was created by this
// store (it carries the lite_issue_versions table of ADR 0001), as opposed to
// a historical upstream SQLite workspace. Any error reads as "not ours".
// lite: lets the legacy-workspace guard admit a workspace bd init just created.
// Callers that must not mistake a busy file for a foreign one use ProbeLiteStore.
func IsLiteStore(path string) bool {
	ok, err := ProbeLiteStore(path)
	return err == nil && ok
}

// ProbeLiteStore is IsLiteStore with the failure kept apart from the answer.
// It returns (false, nil) only when the file opened and has no lite table (or
// is not a SQLite database at all): a definite "not ours". A transient failure
// (another process holds the lock, or the WAL/SHM files are being created or
// removed as the last connection closes) is retried, then returned as an
// error, so a parallel writer can never make a current store look historical
// (company-1bo5.9).
func ProbeLiteStore(path string) (bool, error) {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: fmt.Sprintf("mode=ro&_pragma=busy_timeout(%d)", probeBusyMS)}
	if len(path) > 1 && path[1] == ':' { // Windows drive path: file:/C:/...
		u.Path = "/" + u.Path
	}
	var lastErr error
	for attempt := 0; attempt < probeAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 40 * time.Millisecond)
		}
		ok, err := probeLiteOnce(u.String())
		if err == nil || !transientProbeError(err) {
			return ok, nil
		}
		lastErr = err
	}
	return false, lastErr
}

// Variables so tests can shorten the wait.
var (
	probeAttempts = 8
	probeBusyMS   = 5000
)

// SetProbeTimingForTest shortens the probe's wait; the returned func restores it.
func SetProbeTimingForTest(attempts, busyMS int) (restore func()) {
	a, b := probeAttempts, probeBusyMS
	probeAttempts, probeBusyMS = attempts, busyMS
	return func() { probeAttempts, probeBusyMS = a, b }
}

func probeLiteOnce(dsnStr string) (bool, error) {
	db, err := openDB(dsnStr)
	if err != nil {
		return false, err
	}
	defer db.Close()
	var n int
	err = db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='lite_issue_versions'").Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// transientProbeError reports a SQLite error that says "try again", not "this
// is a different kind of file": busy, locked, cannot open (WAL/SHM racing),
// read-only recovery, I/O error, and a sharing violation on Windows.
func transientProbeError(err error) bool {
	m := strings.ToLower(err.Error())
	for _, s := range []string{"busy", "locked", "unable to open", "readonly", "read-only", "disk i/o", "sharing violation", "being used by another process"} {
		if strings.Contains(m, s) {
			return true
		}
	}
	return false
}
