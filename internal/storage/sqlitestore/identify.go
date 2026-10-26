package sqlitestore

import (
	"context"
	"net/url"
	"path/filepath"
)

// IsLiteStore reports whether the SQLite file at path was created by this
// store (it carries the lite_issue_versions table of ADR 0001), as opposed to
// a historical upstream SQLite workspace. Any error reads as "not ours".
// lite: lets the legacy-workspace guard admit a workspace bd init just created.
func IsLiteStore(path string) bool {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path), RawQuery: "mode=ro"}
	if len(path) > 1 && path[1] == ':' { // Windows drive path: file:/C:/...
		u.Path = "/" + u.Path
	}
	db, err := openDB(u.String())
	if err != nil {
		return false
	}
	defer db.Close()
	var n int
	err = db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='lite_issue_versions'").Scan(&n)
	return err == nil && n > 0
}
