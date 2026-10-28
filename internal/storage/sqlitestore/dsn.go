package sqlitestore

import (
	"net/url"
	"path/filepath"
	"strings"
)

// dsn builds the modernc.org/sqlite DSN for a database file (research R3).
// modernc applies every _pragma on each new connection.
//
//   - busy_timeout(30000): listed FIRST so it is active for every later pragma
//     too. A waiting process sleeps inside SQLite instead of failing with
//     SQLITE_BUSY; a write is milliseconds, but 30 parallel writers queue on one
//     lock and the tail waited more than the old 5 s (~3% failed, company-1bo5.10).
//     Degrade slower, never error.
//   - foreign_keys(1): FK enforcement is off by default in SQLite.
//   - journal_mode(WAL): readers never wait for the writer.
//   - case_sensitive_like(1): LIKE parity with Dolt's binary collation.
//   - _txlock=immediate: writers take the lock up front (no upgrade deadlock).
//   - _timezone=UTC: bound time.Time values are converted to UTC before they are
//     formatted; without it a local-zone time compares against UTC text wrongly.
//   - _time_format=datetime: time.Time is stored as "YYYY-MM-DD HH:MM:SS" (UTC,
//     second precision) like Dolt datetime(0), so strftime and DEFAULT
//     CURRENT_TIMESTAMP rows sort and parse the same way (ADR 0001 section 2).
func dsn(path string) string {
	if strings.HasPrefix(path, "file:") {
		return path
	}
	p := filepath.ToSlash(path)
	// Escape '?' and '#' so a path cannot smuggle DSN options.
	p = (&url.URL{Path: p}).EscapedPath()
	return "file:" + p + "?_pragma=busy_timeout(30000)&_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)&_pragma=case_sensitive_like(1)" +
		"&_txlock=immediate&_time_format=datetime&_timezone=UTC"
}
