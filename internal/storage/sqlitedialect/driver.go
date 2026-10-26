package sqlitedialect

import (
	"context"
	"database/sql"
	"database/sql/driver"

	sqlite "modernc.org/sqlite"
)

// Open returns a *sql.DB over modernc.org/sqlite whose Prepare translates bd's
// canonical MySQL SQL to SQLite (see Translate). It exposes no Execer/Queryer fast
// path, so database/sql routes every statement through Prepare.
// The caller supplies foreign-key enforcement etc. via DSN pragmas.
func Open(dsn string) (*sql.DB, error) {
	return sql.OpenDB(&connector{dsn: dsn}), nil
}

// registered is modernc's process-wide driver instance (the one behind
// sql.Open("sqlite", ...)). It is the instance that carries user-registered
// udfs such as the regexp() the store registers for MySQL's REGEXP; a
// zero-value sqlite.Driver would not see them. sql.Open does not connect.
var registered = func() driver.Driver {
	db, err := sql.Open("sqlite", "")
	if err != nil {
		return &sqlite.Driver{}
	}
	return db.Driver()
}()

// connector opens a fresh modernc connection per Connect on the registered
// driver; connection config travels in the DSN.
type connector struct{ dsn string }

func (c *connector) Connect(_ context.Context) (driver.Conn, error) {
	base, err := registered.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &conn{base: base}, nil
}

func (c *connector) Driver() driver.Driver { return registered }

type conn struct{ base driver.Conn }

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	st, err := c.base.Prepare(Translate(query))
	if err != nil {
		return nil, err
	}
	return wrapStmt(st), nil
}

func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	q := Translate(query)
	if p, ok := c.base.(driver.ConnPrepareContext); ok {
		st, err := p.PrepareContext(ctx, q)
		if err != nil {
			return nil, err
		}
		return wrapStmt(st), nil
	}
	st, err := c.base.Prepare(q)
	if err != nil {
		return nil, err
	}
	return wrapStmt(st), nil
}

func (c *conn) Close() error { return c.base.Close() }

//nolint:staticcheck // SA1019: driver.Conn requires the deprecated Begin; BeginTx forwarded below.
func (c *conn) Begin() (driver.Tx, error) { return c.base.Begin() }

func (c *conn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if b, ok := c.base.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return c.base.Begin() //nolint:staticcheck
}

func (c *conn) ResetSession(ctx context.Context) error {
	if rs, ok := c.base.(driver.SessionResetter); ok {
		return rs.ResetSession(ctx)
	}
	return nil
}

func (c *conn) IsValid() bool {
	if v, ok := c.base.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}
