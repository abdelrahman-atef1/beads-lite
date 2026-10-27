package sqlitedialect

import (
	"context"
	"database/sql/driver"
	"reflect"
	"time"
)

// SQLite hands back a bare aggregate such as MAX(created_at) as TEXT with no
// declared column type, so database/sql cannot scan it into a time.Time or
// sql.NullTime the way Dolt's driver allows. wrapStmt decodes such values:
// only columns with an empty declared type (expressions) whose text is exactly
// a known datetime text form (see sqliteDatetimeLayouts) become time.Time.
// Plain columns are untouched, so a datetime column scanned into a string
// still reads back as stored.
const sqliteDatetime = "2006-01-02 15:04:05"

// sqliteDatetimeLayouts are the text forms a datetime column holds in a real
// store: the native write format, plus RFC 3339 (with or without fractional
// seconds) left by migrated/imported rows. A live store mixes them, so
// MAX(created_at) can come back in either (company-1bo5 P0: "...T05:42:51Z").
var sqliteDatetimeLayouts = []string{sqliteDatetime, time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02T15:04:05.999999999"}

func parseSQLiteDatetime(s string) (time.Time, bool) {
	if len(s) < len(sqliteDatetime) {
		return time.Time{}, false
	}
	for _, l := range sqliteDatetimeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

func wrapStmt(st driver.Stmt) driver.Stmt { return &stmt{base: st} }

type stmt struct{ base driver.Stmt }

func (s *stmt) Close() error  { return s.base.Close() }
func (s *stmt) NumInput() int { return s.base.NumInput() }

//nolint:staticcheck // SA1019: driver.Stmt requires the deprecated Exec/Query.
func (s *stmt) Exec(args []driver.Value) (driver.Result, error) { return s.base.Exec(args) }

//nolint:staticcheck // SA1019: see Exec.
func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	r, err := s.base.Query(args)
	if err != nil {
		return nil, err
	}
	return wrapRows(r), nil
}

func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	if e, ok := s.base.(driver.StmtExecContext); ok {
		return e.ExecContext(ctx, args)
	}
	return s.base.Exec(namedToValues(args)) //nolint:staticcheck
}

func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	var r driver.Rows
	var err error
	if q, ok := s.base.(driver.StmtQueryContext); ok {
		r, err = q.QueryContext(ctx, args)
	} else {
		r, err = s.base.Query(namedToValues(args)) //nolint:staticcheck
	}
	if err != nil {
		return nil, err
	}
	return wrapRows(r), nil
}

func namedToValues(args []driver.NamedValue) []driver.Value {
	out := make([]driver.Value, len(args))
	for i, a := range args {
		out[i] = a.Value
	}
	return out
}

type rows struct {
	driver.Rows
	// timeCols marks expression columns (no declared type) to decode.
	timeCols []bool
}

func wrapRows(r driver.Rows) driver.Rows {
	tn, ok := r.(driver.RowsColumnTypeDatabaseTypeName)
	if !ok {
		return r
	}
	n := len(r.Columns())
	w := &rows{Rows: r, timeCols: make([]bool, n)}
	any := false
	for i := 0; i < n; i++ {
		if tn.ColumnTypeDatabaseTypeName(i) == "" {
			w.timeCols[i], any = true, true
		}
	}
	if !any {
		return r
	}
	return w
}

func (r *rows) Next(dest []driver.Value) error {
	if err := r.Rows.Next(dest); err != nil {
		return err
	}
	for i, v := range dest {
		if i >= len(r.timeCols) || !r.timeCols[i] {
			continue
		}
		if s, ok := v.(string); ok {
			if t, ok := parseSQLiteDatetime(s); ok {
				dest[i] = t
			}
		}
	}
	return nil
}

func (r *rows) ColumnTypeDatabaseTypeName(i int) string {
	return r.Rows.(driver.RowsColumnTypeDatabaseTypeName).ColumnTypeDatabaseTypeName(i)
}

func (r *rows) ColumnTypeScanType(i int) reflect.Type {
	if s, ok := r.Rows.(driver.RowsColumnTypeScanType); ok {
		return s.ColumnTypeScanType(i)
	}
	return reflect.TypeOf("")
}

func (r *rows) ColumnTypeNullable(i int) (bool, bool) {
	if s, ok := r.Rows.(driver.RowsColumnTypeNullable); ok {
		return s.ColumnTypeNullable(i)
	}
	return false, false
}
