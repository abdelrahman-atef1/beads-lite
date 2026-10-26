package sqlitelite

// W8 loader (T020). Writes a NEW sqlite database from what the extractor
// read. It never opens, copies or touches the Dolt store: its only inputs are
// bytes in memory (Input), so what the extractor can or cannot read is a
// separate question from how it is loaded.
//
// Pluggable input: the loader consumes Input, not Result. InputFromResult maps
// the extractor's Result; anything the extractor cannot read through bd 1.3.1
// (local_metadata, leases, journal, provenance, ...) is passed in
// Input.Tables as generic rows keyed by column name. Every table and column
// name is checked against the live schema before use, so an unknown name is an
// error, never SQL built from input.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/steveyegge/beads/internal/storage/kvkeys"
	"github.com/steveyegge/beads/internal/storage/sqlitestore"
	"github.com/steveyegge/beads/internal/types"
)

// Input is everything the loader needs.
type Input struct {
	// IssuesJSONL is `bd export` output: one JSON object per line.
	IssuesJSONL []byte
	// Config is `bd config list --json`.
	Config map[string]any
	// Beads carries history snapshots and audit events per bead.
	Beads []*BeadHistory
	// Tables holds extra rows by table name (local_metadata, leases,
	// provenance_events, bd_events_journal, custom_statuses, ...). A table
	// listed here replaces the loader's own derivation for that table.
	Tables map[string][]map[string]any
}

// InputFromResult adapts the extractor's output.
func InputFromResult(r *Result) *Input {
	in := &Input{IssuesJSONL: r.IssuesJSONL, Config: r.Config, Beads: r.Beads, Tables: map[string][]map[string]any{}}
	for t, rows := range r.Tables {
		in.Tables[t] = rows
	}
	return in
}

// LoadReport counts what was written.
type LoadReport struct {
	Issues       int            `json:"issues"`
	Memories     int            `json:"memories"`
	Versions     int            `json:"versions"`
	Rows         map[string]int `json:"rows"` // table -> rows written
	DroppedKeys  map[string]int `json:"dropped_keys,omitempty"`
	ExtraVersion []string       `json:"extra_version,omitempty"` // beads whose export differed from newest snapshot
}

// issueKeyAlias maps an export JSON key to its issues column when the names differ.
var issueKeyAlias = map[string]string{"timeout": "timeout_ns"}

// notIssueColumns are export keys that live in other tables or are derived.
var notIssueColumns = map[string]bool{
	"_type": true, "labels": true, "dependencies": true, "comments": true,
	"dependency_count": true, "dependent_count": true, "comment_count": true,
	"lease_expires_at": true, "heartbeat_at": true, "lease_granted_node": true,
	"dependents": true, "parent": true,
}

var largeFive = []string{"description", "design", "acceptance_criteria", "notes", "metadata"}

// Load writes the database at dest, which must not exist. On any error the
// partial file is removed. The caller owns the rename into place.
func Load(ctx context.Context, in *Input, dest string) (*LoadReport, error) {
	if _, err := os.Stat(dest); err == nil {
		return nil, fmt.Errorf("sqlitelite: %s already exists; the loader only creates new databases", dest)
	}
	rep, err := load(ctx, in, dest)
	if err != nil {
		for _, s := range []string{"", "-wal", "-shm", "-journal"} {
			_ = os.Remove(dest + s)
		}
		return nil, err
	}
	return rep, nil
}

type issueRec struct {
	id    string
	raw   map[string]json.RawMessage
	cols  map[string]any
	typed types.Issue
}

func load(ctx context.Context, in *Input, dest string) (*LoadReport, error) {
	st, err := sqlitestore.Open(ctx, dest, sqlitestore.Options{Create: true, Actor: "lite-migrate"})
	if err != nil {
		return nil, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = st.Close()
		}
	}()
	// Versions are written explicitly below from extracted history; the live
	// before-commit hook is not registered on this store.

	rep := &LoadReport{Rows: map[string]int{}, DroppedKeys: map[string]int{}}
	schema, err := tableColumns(ctx, st.DB())
	if err != nil {
		return nil, err
	}
	issueCols := schema["issues"]

	var recs []*issueRec
	var mems [][2]string
	seen := map[string]bool{}
	for n, ln := range bytes.Split(in.IssuesJSONL, []byte("\n")) {
		ln = bytes.TrimSpace(ln)
		if len(ln) == 0 {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(ln, &raw); err != nil {
			return nil, fmt.Errorf("sqlitelite: export line %d: %w", n+1, err)
		}
		if t := rawString(raw["_type"]); t != "" && t != "issue" {
			if t == "memory" { // memories live in config as kv.memory.<key>
				mems = append(mems, [2]string{kvkeys.MemoryConfigKeyPrefix + rawString(raw["key"]), rawString(raw["value"])})
			}
			rep.Memories++
			continue
		}
		var typed types.Issue
		if err := json.Unmarshal(ln, &typed); err != nil {
			return nil, fmt.Errorf("sqlitelite: export line %d: %w", n+1, err)
		}
		if typed.ID == "" || seen[typed.ID] {
			return nil, fmt.Errorf("sqlitelite: export line %d: missing or duplicate id %q", n+1, typed.ID)
		}
		seen[typed.ID] = true
		cols, dropped := issueColumns(raw, issueCols)
		for _, k := range dropped {
			rep.DroppedKeys[k]++
		}
		cols["content_hash"] = typed.ComputeContentHash()
		recs = append(recs, &issueRec{id: typed.ID, raw: raw, cols: cols, typed: typed})
	}
	if len(rep.DroppedKeys) == 0 {
		rep.DroppedKeys = nil
	}

	hist := map[string]*BeadHistory{}
	for _, b := range in.Beads {
		hist[b.ID] = b
	}

	err = st.RunInTx(ctx, func(tx *sql.Tx) error {
		for _, r := range recs {
			if err := insertRow(ctx, tx, "issues", r.cols); err != nil {
				return fmt.Errorf("issue %s: %w", r.id, err)
			}
			rep.Rows["issues"]++
		}
		rep.Issues = len(recs)
		for _, r := range recs {
			if err := loadChildren(ctx, tx, r, seen, rep); err != nil {
				return fmt.Errorf("issue %s: %w", r.id, err)
			}
			if err := loadEvents(ctx, tx, r.id, hist[r.id], in, rep); err != nil {
				return fmt.Errorf("issue %s events: %w", r.id, err)
			}
			if err := loadVersions(ctx, tx, r, hist[r.id], issueCols, rep); err != nil {
				return fmt.Errorf("issue %s history: %w", r.id, err)
			}
		}
		if err := loadLeases(ctx, tx, recs, in, rep); err != nil {
			return err
		}
		if err := loadConfig(ctx, tx, in.Config, rep); err != nil {
			return err
		}
		for _, m := range mems {
			if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO config (`key`, value) VALUES (?, ?)", m[0], m[1]); err != nil {
				return err
			}
		}
		return loadTables(ctx, tx, in.Tables, schema, rep)
	})
	if err != nil {
		return nil, err
	}
	// Leave one self-contained file: flush the WAL into the main file.
	if _, err := st.DB().ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return nil, fmt.Errorf("sqlitelite: checkpoint: %w", err)
	}
	closed = true
	if err := st.Close(); err != nil {
		return nil, err
	}
	return rep, nil
}

func rawString(r json.RawMessage) string {
	var s string
	_ = json.Unmarshal(r, &s)
	return s
}

// issueColumns converts one export object into column values. Timestamps are
// normalised to UTC RFC3339; bools to 0/1; objects/arrays to compact JSON text.
func issueColumns(raw map[string]json.RawMessage, valid map[string]bool) (map[string]any, []string) {
	cols := map[string]any{}
	var dropped []string
	for k, v := range raw {
		if notIssueColumns[k] {
			continue
		}
		c := k
		if a, ok := issueKeyAlias[k]; ok {
			c = a
		}
		if !valid[c] {
			dropped = append(dropped, k)
			continue
		}
		cols[c] = jsonToSQL(c, v)
	}
	for _, c := range []string{"title", "description", "design", "acceptance_criteria", "notes"} {
		if _, ok := cols[c]; !ok {
			cols[c] = ""
		}
	}
	if _, ok := cols["metadata"]; !ok {
		cols["metadata"] = "{}"
	}
	sort.Strings(dropped)
	return cols, dropped
}

func jsonToSQL(col string, v json.RawMessage) any {
	v = bytes.TrimSpace(v)
	if len(v) == 0 || string(v) == "null" {
		return nil
	}
	switch v[0] {
	case '"':
		var s string
		_ = json.Unmarshal(v, &s)
		if isTimeCol(col) {
			return normTime(s)
		}
		return s
	case 't':
		return int64(1)
	case 'f':
		return int64(0)
	case '{', '[':
		var buf bytes.Buffer
		if json.Compact(&buf, v) != nil {
			return string(v)
		}
		return buf.String()
	}
	var n json.Number
	d := json.NewDecoder(bytes.NewReader(v))
	d.UseNumber()
	_ = d.Decode(&n)
	if i, err := n.Int64(); err == nil {
		return i
	}
	f, _ := n.Float64()
	return f
}

func isTimeCol(c string) bool {
	return strings.HasSuffix(c, "_at") || c == "last_activity" || c == "defer_until"
}

// normTime returns RFC3339 UTC when s parses, else s unchanged.
func normTime(s string) string {
	for _, l := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}

func insertRow(ctx context.Context, tx *sql.Tx, table string, cols map[string]any) error {
	return insertRowVerb(ctx, tx, "INSERT", table, cols)
}

func insertRowVerb(ctx context.Context, tx *sql.Tx, verb, table string, cols map[string]any) error {
	names := make([]string, 0, len(cols))
	for c := range cols {
		names = append(names, c)
	}
	sort.Strings(names)
	args := make([]any, len(names))
	ph := make([]string, len(names))
	q := make([]string, len(names))
	for i, c := range names {
		args[i], ph[i], q[i] = cols[c], "?", "`"+c+"`"
	}
	_, err := tx.ExecContext(ctx, verb+" INTO `"+table+"` ("+strings.Join(q, ",")+") VALUES ("+strings.Join(ph, ",")+")", args...)
	return err
}

func tableColumns(ctx context.Context, db *sql.DB) (map[string]map[string]bool, error) {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return nil, err
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			rows.Close()
			return nil, err
		}
		tables = append(tables, n)
	}
	rows.Close()
	out := map[string]map[string]bool{}
	for _, t := range tables {
		r, err := db.QueryContext(ctx, "SELECT name FROM pragma_table_info('"+strings.ReplaceAll(t, "'", "''")+"')")
		if err != nil {
			return nil, err
		}
		m := map[string]bool{}
		for r.Next() {
			var n string
			if err := r.Scan(&n); err != nil {
				r.Close()
				return nil, err
			}
			m[n] = true
		}
		r.Close()
		out[t] = m
	}
	return out, nil
}

// uuidFrom builds a stable UUID-shaped id (v5-like) from parts.
func uuidFrom(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	h := hex.EncodeToString(sum[:16])
	return h[0:8] + "-" + h[8:12] + "-5" + h[13:16] + "-a" + h[17:20] + "-" + h[20:32]
}

func loadChildren(ctx context.Context, tx *sql.Tx, r *issueRec, ids map[string]bool, rep *LoadReport) error {
	var labels []string
	if v, ok := r.raw["labels"]; ok {
		if err := json.Unmarshal(v, &labels); err != nil {
			return fmt.Errorf("labels: %w", err)
		}
	}
	for _, l := range labels {
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO labels (issue_id, label) VALUES (?, ?)", r.id, l); err != nil {
			return err
		}
		rep.Rows["labels"]++
	}
	var deps []struct {
		IssueID   string          `json:"issue_id"`
		DependsOn string          `json:"depends_on_id"`
		Type      string          `json:"type"`
		CreatedAt string          `json:"created_at"`
		CreatedBy string          `json:"created_by"`
		Metadata  json.RawMessage `json:"metadata"`
		ThreadID  string          `json:"thread_id"`
	}
	if v, ok := r.raw["dependencies"]; ok {
		if err := json.Unmarshal(v, &deps); err != nil {
			return fmt.Errorf("dependencies: %w", err)
		}
	}
	for _, d := range deps {
		row := map[string]any{
			"id": uuidFrom("dep", r.id, d.DependsOn, d.Type), "issue_id": r.id,
			"type": d.Type, "created_at": normTime(d.CreatedAt), "created_by": d.CreatedBy,
			"thread_id": d.ThreadID, "metadata": "{}",
		}
		if len(d.Metadata) > 0 {
			var s string
			if json.Unmarshal(d.Metadata, &s) == nil {
				row["metadata"] = s
			} else if string(d.Metadata) != "null" {
				row["metadata"] = string(d.Metadata)
			}
		}
		switch {
		case strings.HasPrefix(d.DependsOn, "external:"):
			row["depends_on_external"] = d.DependsOn
		case ids[d.DependsOn]:
			row["depends_on_issue_id"] = d.DependsOn
		default:
			// Target is neither an exported issue nor external (a wisp or a
			// dangling edge). Keep it as external text so nothing is lost
			// silently; verify counts it by type.
			row["depends_on_external"] = d.DependsOn
		}
		if err := insertRow(ctx, tx, "dependencies", row); err != nil {
			return fmt.Errorf("dependency %s -> %s: %w", r.id, d.DependsOn, err)
		}
		rep.Rows["dependencies"]++
	}
	var comments []struct {
		ID        string `json:"id"`
		Author    string `json:"author"`
		Text      string `json:"text"`
		CreatedAt string `json:"created_at"`
	}
	if v, ok := r.raw["comments"]; ok {
		if err := json.Unmarshal(v, &comments); err != nil {
			return fmt.Errorf("comments: %w", err)
		}
	}
	for i, c := range comments {
		id := c.ID
		if id == "" {
			id = uuidFrom("comment", r.id, fmt.Sprint(i), c.CreatedAt, c.Text)
		}
		if err := insertRow(ctx, tx, "comments", map[string]any{
			"id": id, "issue_id": r.id, "author": c.Author, "text": c.Text, "created_at": normTime(c.CreatedAt),
		}); err != nil {
			return fmt.Errorf("comment %s: %w", id, err)
		}
		rep.Rows["comments"]++
	}
	return nil
}

func loadEvents(ctx context.Context, tx *sql.Tx, id string, b *BeadHistory, in *Input, rep *LoadReport) error {
	if b == nil || len(b.Events) == 0 {
		return nil
	}
	if _, over := in.Tables["events"]; over {
		return nil
	}
	var evs []map[string]any
	if err := json.Unmarshal(b.Events, &evs); err != nil {
		return err
	}
	for i, e := range evs {
		row := map[string]any{"issue_id": id}
		for _, k := range []string{"id", "event_type", "actor", "old_value", "new_value", "comment", "created_at"} {
			if v, ok := e[k]; ok && v != nil {
				row[k] = v
			}
		}
		if s, _ := row["created_at"].(string); s != "" {
			row["created_at"] = normTime(s)
		}
		if _, ok := row["id"]; !ok {
			row["id"] = uuidFrom("event", id, fmt.Sprint(i))
		}
		if _, ok := row["actor"]; !ok {
			row["actor"] = ""
		}
		if _, ok := row["event_type"]; !ok {
			return fmt.Errorf("event %d has no event_type", i)
		}
		if err := insertRow(ctx, tx, "events", row); err != nil {
			return err
		}
		rep.Rows["events"]++
	}
	return nil
}

// snapshot is one `bd history <id> --json` row.
type snapshot struct {
	CommitHash string                     `json:"CommitHash"`
	Committer  string                     `json:"Committer"`
	CommitDate string                     `json:"CommitDate"`
	Issue      map[string]json.RawMessage `json:"Issue"`
}

// loadVersions seeds lite_issue_versions, oldest first, in the exact shape
// sqlitestore.writeVersions produces (ADR 0001 section 4). History from bd is
// newest first. If the newest snapshot differs from the exported row the
// exported row is appended as a final version, so History()'s head always
// equals the current issue.
func loadVersions(ctx context.Context, tx *sql.Tx, r *issueRec, b *BeadHistory, valid map[string]bool, rep *LoadReport) error {
	var snaps []snapshot
	if b != nil && len(b.Snapshots) > 0 {
		if err := json.Unmarshal(b.Snapshots, &snaps); err != nil {
			return err
		}
	}
	write := func(committedAt, committer string, cols map[string]any) error {
		scalars, bundle := splitVersion(cols)
		body, _ := json.Marshal(bundle)
		sum := sha256.Sum256(body)
		hash := hex.EncodeToString(sum[:])
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO lite_version_text (hash, body) VALUES (?, ?)", hash, string(body)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO lite_issue_versions (issue_id, committed_at, committer, scalars, text_hash) VALUES (?, ?, ?, ?, ?)",
			r.id, committedAt, committer, string(scalars), hash); err != nil {
			return err
		}
		rep.Versions++
		return nil
	}
	var newest map[string]any
	for i := len(snaps) - 1; i >= 0; i-- {
		s := snaps[i]
		cols, _ := issueColumns(s.Issue, valid)
		when := normTime(s.CommitDate)
		if err := write(when, s.Committer, cols); err != nil {
			return err
		}
		newest = cols
	}
	cur := map[string]any{}
	for k, v := range r.cols {
		if k != "content_hash" {
			cur[k] = v
		}
	}
	if newest == nil || !sameVersion(newest, cur) {
		if newest != nil {
			rep.ExtraVersion = append(rep.ExtraVersion, r.id)
		}
		upd, _ := r.cols["updated_at"].(string)
		if upd == "" {
			upd = time.Now().UTC().Format(time.RFC3339)
		}
		return write(upd, "lite-migrate", cur)
	}
	return nil
}

type textBundle struct {
	Description        string `json:"description"`
	Design             string `json:"design"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
	Notes              string `json:"notes"`
	Metadata           string `json:"metadata"`
}

// splitVersion mirrors sqlitestore.snapshotIssue: scalars omit NULL and ""
// values and the large five; the five go into the text bundle.
func splitVersion(cols map[string]any) ([]byte, textBundle) {
	var b textBundle
	sc := map[string]any{}
	str := func(v any) string { s, _ := v.(string); return s }
	for c, v := range cols {
		switch c {
		case "description":
			b.Description = str(v)
		case "design":
			b.Design = str(v)
		case "acceptance_criteria":
			b.AcceptanceCriteria = str(v)
		case "notes":
			b.Notes = str(v)
		case "metadata":
			b.Metadata = str(v)
		default:
			if v == nil || v == "" {
				continue
			}
			sc[c] = v
		}
	}
	out, _ := json.Marshal(sc)
	return out, b
}

func sameVersion(a, b map[string]any) bool {
	sa, ba := splitVersion(a)
	sb, bb := splitVersion(b)
	return bytes.Equal(sa, sb) && ba == bb
}

func loadLeases(ctx context.Context, tx *sql.Tx, recs []*issueRec, in *Input, rep *LoadReport) error {
	if _, over := in.Tables["leases"]; over {
		return nil
	}
	for _, r := range recs {
		exp := rawString(r.raw["lease_expires_at"])
		if exp == "" {
			continue
		}
		hb := rawString(r.raw["heartbeat_at"])
		if hb == "" {
			hb = exp
		}
		holder, _ := r.cols["assignee"].(string)
		if err := insertRow(ctx, tx, "leases", map[string]any{
			"issue_id": r.id, "holder": holder, "granted_at": normTime(hb),
			"lease_expires_at": normTime(exp), "heartbeat_at": normTime(hb),
			"granted_node": rawString(r.raw["lease_granted_node"]),
		}); err != nil {
			return fmt.Errorf("lease %s: %w", r.id, err)
		}
		rep.Rows["leases"]++
	}
	return nil
}

func loadConfig(ctx context.Context, tx *sql.Tx, cfg map[string]any, rep *LoadReport) error {
	keys := make([]string, 0, len(cfg))
	for k := range cfg {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var v string
		switch x := cfg[k].(type) {
		case string:
			v = x
		case nil:
			continue
		default:
			b, _ := json.Marshal(x)
			v = string(b)
		}
		if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO config (`key`, value) VALUES (?, ?)", k, v); err != nil {
			return err
		}
		rep.Rows["config"]++
	}
	return nil
}

// loadTables inserts caller-supplied rows after validating every name.
func loadTables(ctx context.Context, tx *sql.Tx, tabs map[string][]map[string]any, schema map[string]map[string]bool, rep *LoadReport) error {
	names := make([]string, 0, len(tabs))
	for t := range tabs {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		cols, ok := schema[t]
		if !ok || strings.HasPrefix(t, "sqlite_") {
			return fmt.Errorf("sqlitelite: Input.Tables: unknown table %q", t)
		}
		for i, row := range tabs[t] {
			clean := map[string]any{}
			for c, v := range row {
				if !cols[c] {
					return fmt.Errorf("sqlitelite: Input.Tables[%s] row %d: unknown column %q", t, i, c)
				}
				if s, ok := v.(string); ok && isTimeCol(c) {
					v = normTime(s)
				}
				clean[c] = v
			}
			if err := insertRowVerb(ctx, tx, "INSERT OR REPLACE", t, clean); err != nil { // extracted rows win over schema seeds
				return fmt.Errorf("sqlitelite: Input.Tables[%s] row %d: %w", t, i, err)
			}
			rep.Rows[t]++
		}
	}
	return nil
}

// ErrUncovered is returned by CheckUncovered when a table the extractor could
// not read holds or may hold rows.
var ErrUncovered = errors.New("sqlitelite: tables the extractor could not cover")

// CheckUncovered is the migrate gate: every table the extractor listed as
// unreadable, unread or non-empty must be named in covered (supplied through
// Input.Tables or deliberately accepted). Anything else stops the switch and
// the project stays on Dolt.
func CheckUncovered(r *Result, covered []string) error {
	ok := map[string]bool{}
	for _, c := range covered {
		ok[c] = true
	}
	var bad []string
	for _, e := range r.Nonzero() {
		if !ok[e[:strings.IndexByte(e, '=')]] {
			bad = append(bad, e)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w: %s", ErrUncovered, strings.Join(bad, "; "))
	}
	return nil
}
