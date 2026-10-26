package sqlitedialect

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// lite: pins the contract Dolt callers rely on. An expression column such as
// COALESCE(created_at,”) comes back from SQLite as bare text; wrapRows turns
// the store's "YYYY-MM-DD HH:MM:SS" text into time.Time, and database/sql then
// renders a time.Time scanned into a string as RFC3339. A declared DATETIME column (and
// RFC3339 text the store wrote) also reads back as RFC3339.
func TestExpressionTimeScansAsRFC3339String(t *testing.T) {
	db, err := Open("file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "r.db")))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, created_at DATETIME)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO t VALUES (1, '2026-10-25 07:51:45'), (2, '2026-10-25T07:51:45Z'), (3, NULL)"); err != nil {
		t.Fatal(err)
	}
	var coalesced, plain string
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(created_at,''), created_at FROM t WHERE id = 1").Scan(&coalesced, &plain); err != nil {
		t.Fatal(err)
	}
	if want := "2026-10-25T07:51:45Z"; coalesced != want {
		t.Fatalf("COALESCE(created_at,'') scanned into string = %q, want RFC3339 %q", coalesced, want)
	}
	if _, err := time.Parse(time.RFC3339, coalesced); err != nil {
		t.Fatalf("not RFC3339: %v", err)
	}
	if plain != "2026-10-25T07:51:45Z" { // modernc parses declared DATETIME columns itself
		t.Fatalf("plain column changed: %q", plain)
	}
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(created_at,'') FROM t WHERE id = 2").Scan(&coalesced); err != nil || coalesced != "2026-10-25T07:51:45Z" {
		t.Fatalf("RFC3339 text: %q %v", coalesced, err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(created_at,'') FROM t WHERE id = 3").Scan(&coalesced); err != nil || coalesced != "" {
		t.Fatalf("NULL: %q %v", coalesced, err)
	}
}
