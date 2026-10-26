package sqlitestore

import (
	"net/url"
	"path/filepath"
	"strings"
)

// dsn builds the modernc.org/sqlite DSN for a database file (research R3).
// modernc applies every _pragma on each new connection.
//
//   - foreign_keys(1): FK enforcement is off by default in SQLite.
//   - journal_mode(WAL): readers never wait for the writer.
//   - busy_timeout(5000): a second process waits up to 5 s inside SQLite instead
//     of failing with SQLITE_BUSY; a write is milliseconds.
//   - case_sensitive_like(1): LIKE parity with Dolt's binary collation.
//   - _txlock=immediate: writers take the lock up front (no upgrade deadlock).
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
	return "file:" + p + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)&_pragma=case_sensitive_like(1)" +
		"&_txlock=immediate&_time_format=datetime"
}
