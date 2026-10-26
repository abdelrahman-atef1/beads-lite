// Package sqlitelite moves a Dolt-backed beads store into the SQLite store.
//
// This file is the W8 extractor (ADR 0003, rebuilt from the first T019 build).
// It works on a COPY of the project folder and never touches the source:
//
//   - upstream bd 1.3.1 (`export`, `config list --json`) supplies issues,
//     comments, dependencies, labels and config in the upstream JSONL format;
//   - the pinned Dolt CLI 2.2.4 reads the embedded Dolt repository directly
//     (`dolt sql -r json`, statements on stdin) for version history (one
//     entry per change, from dolt_diff_issues), events, and every other
//     non-empty table the export does not carry.
//
// Telemetry stays off: every Dolt call runs with DOLT_ROOT_PATH pointing at a
// folder inside WorkDir whose config_global.json disables metrics and the
// version check. The owner's ~/.dolt is never read or written.
package sqlitelite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// DefaultDoltBin is the Dolt CLI installed by the CTO (ADR 0003).
const DefaultDoltBin = "E:/Tools/dolt-2.2.4/dolt-windows-amd64/bin/dolt.exe"

// Options configures an extraction.
type Options struct {
	// BdBin is the upstream bd 1.3.1 executable (never the fork).
	BdBin string
	// DoltBin is the pinned Dolt CLI (default: $BD_LITE_DOLT, else DefaultDoltBin).
	DoltBin string
	// StoreCopy is a COPY of the project folder (the folder that contains
	// .beads). It is only read from.
	StoreCopy string
	// WorkDir holds the working copy and the Dolt config. Must be empty or absent.
	WorkDir string
	// CallTimeout bounds one external call (default 10 min).
	CallTimeout time.Duration
	// OracleBeads is how many beads are compared against upstream
	// `bd history` (default 6; negative disables).
	OracleBeads int
	// Progress, if set, is called with a short line now and then.
	Progress func(string)
}

// BeadHistory is what was read for one bead.
type BeadHistory struct {
	ID        string
	UpdatedAt string
	// Snapshots is a JSON array in upstream `bd history --json` shape,
	// newest first: [{CommitHash,Committer,CommitDate,Issue}], one element
	// per Dolt commit that changed the bead (UTC dates).
	Snapshots json.RawMessage
	// Cols is a JSON array parallel to Snapshots (newest first): every issues
	// column of that version, keyed by column name. Snapshots carries only the
	// fields of types.Issue; Cols is what the loader compares with the export.
	Cols json.RawMessage
	// Events is a JSON array in upstream `bd history --events --json` shape
	// (types.Event), newest first.
	Events json.RawMessage
	// Provenance is always 0 now: provenance_events rows arrive in Result.Tables.
	Provenance int
}

// Result is the full extraction.
type Result struct {
	IssuesJSONL []byte         // `bd export` output, byte for byte
	Config      map[string]any // `bd config list --json`
	Beads       []*BeadHistory // one per exported issue, export order
	// Uncovered maps every base table that `bd export` does not carry (and is
	// not bookkeeping) to its row count.
	Uncovered map[string]string
	// Tables holds the rows of every non-empty table in Uncovered, keyed by
	// column name (feed to Input.Tables). Datetimes are UTC "2006-01-02 15:04:05".
	Tables map[string][]map[string]any
	Timing Timing
}

// Timing records wall time per stage, in milliseconds.
type Timing struct {
	Issues   int   `json:"issues"`
	Versions int   `json:"versions"`
	Events   int   `json:"events"`
	CopyMs   int64 `json:"copy_ms"`
	DoltMs   int64 `json:"dolt_ms"`
	ExportMs int64 `json:"export_ms"`
	ConfigMs int64 `json:"config_ms"`
	OracleMs int64 `json:"oracle_ms"`
	TotalMs  int64 `json:"total_ms"`
	BdCalls  int   `json:"bd_calls"`
	Oracle   int   `json:"oracle_beads"`
}

// Extractor runs the extraction. Create with New, call Extract, then Cleanup.
type Extractor struct {
	o     Options
	root  string // working copy of the project folder
	dolt  string // dolt database dir inside the copy
	doltR string // DOLT_ROOT_PATH
	calls int
}

// New validates options.
func New(o Options) (*Extractor, error) {
	if o.BdBin == "" || o.StoreCopy == "" || o.WorkDir == "" {
		return nil, errors.New("sqlitelite: BdBin, StoreCopy and WorkDir are required")
	}
	if st, err := os.Stat(filepath.Join(o.StoreCopy, ".beads")); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("sqlitelite: %s has no .beads folder", o.StoreCopy)
	}
	if o.DoltBin == "" {
		o.DoltBin = os.Getenv("BD_LITE_DOLT")
	}
	if o.DoltBin == "" {
		o.DoltBin = DefaultDoltBin
	}
	if o.CallTimeout <= 0 {
		o.CallTimeout = 10 * time.Minute
	}
	if o.OracleBeads == 0 {
		o.OracleBeads = 6
	}
	return &Extractor{o: o}, nil
}

func (e *Extractor) say(f string, a ...any) {
	if e.o.Progress != nil {
		e.o.Progress(fmt.Sprintf(f, a...))
	}
}

// run executes bd against the working copy. Returns stdout.
func (e *Extractor) run(args ...string) ([]byte, error) {
	e.calls++
	ctx, cancel := context.WithTimeout(context.Background(), e.o.CallTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, e.o.BdBin, args...)
	c.Dir = e.root
	c.Env = append(os.Environ(),
		"BEADS_DIR="+filepath.Join(e.root, ".beads"),
		"BD_DISABLE_METRICS=1", "NO_COLOR=1",
		"BEADS_ACTOR=lite-extract", "BD_ACTOR=lite-extract")
	hideWindow(c)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("bd %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(tail(errb.String(), 400)))
	}
	return out.Bytes(), nil
}

// runDolt executes the pinned Dolt CLI in the copied database with telemetry
// disabled, feeding sql on stdin, and returns stdout.
func (e *Extractor) runDolt(sql string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.o.CallTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, e.o.DoltBin, args...)
	c.Dir = e.dolt
	c.Env = append(os.Environ(), "DOLT_ROOT_PATH="+e.doltR, "NO_COLOR=1")
	c.Stdin = strings.NewReader(sql)
	hideWindow(c)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	if err := c.Run(); err != nil {
		return nil, fmt.Errorf("dolt %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(tail(errb.String(), 400)))
	}
	return out.Bytes(), nil
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// Extract does the full read.
func (e *Extractor) Extract() (*Result, error) {
	t0 := time.Now()
	r := &Result{Uncovered: map[string]string{}, Tables: map[string][]map[string]any{}}

	if err := e.prepare(); err != nil {
		return nil, err
	}
	r.Timing.CopyMs = time.Since(t0).Milliseconds()
	e.say("copied in %d ms", r.Timing.CopyMs)

	if err := e.readExportAndConfig(r); err != nil {
		return nil, err
	}
	e.say("export %d ms, config %d ms, %d issues", r.Timing.ExportMs, r.Timing.ConfigMs, len(r.Beads))

	td := time.Now()
	if err := e.readDolt(r); err != nil {
		return nil, err
	}
	r.Timing.DoltMs = time.Since(td).Milliseconds()
	e.say("dolt %d ms, %d versions, %d events", r.Timing.DoltMs, r.Timing.Versions, r.Timing.Events)

	r.Timing.Issues = len(r.Beads)
	if e.o.OracleBeads > 0 {
		to := time.Now()
		n, err := e.oracle(r)
		r.Timing.OracleMs = time.Since(to).Milliseconds()
		r.Timing.Oracle = n
		if err != nil {
			r.Timing.BdCalls = e.calls
			r.Timing.TotalMs = time.Since(t0).Milliseconds()
			return r, err
		}
	}
	r.Timing.BdCalls = e.calls
	r.Timing.TotalMs = time.Since(t0).Milliseconds()
	return r, nil
}

// prepare makes the working copy and the Dolt config.
func (e *Extractor) prepare() error {
	if ents, err := os.ReadDir(e.o.WorkDir); err == nil && len(ents) > 0 {
		return fmt.Errorf("sqlitelite: WorkDir %s is not empty", e.o.WorkDir)
	}
	e.root = filepath.Join(e.o.WorkDir, "copy")
	e.doltR = filepath.Join(e.o.WorkDir, "dolt-root")
	if err := os.MkdirAll(filepath.Join(e.doltR, ".dolt"), 0o755); err != nil {
		return err
	}
	cfg := `{"metrics.disabled":"true","versioncheck.disabled":"true"}`
	if err := os.WriteFile(filepath.Join(e.doltR, ".dolt", "config_global.json"), []byte(cfg), 0o644); err != nil {
		return err
	}
	if err := copyBeads(e.o.StoreCopy, e.root); err != nil {
		return err
	}
	base := filepath.Join(e.root, ".beads", "embeddeddolt")
	ents, err := os.ReadDir(base)
	if err != nil {
		return fmt.Errorf("sqlitelite: no embeddeddolt folder: %w", err)
	}
	for _, d := range ents {
		if d.IsDir() {
			if st, err := os.Stat(filepath.Join(base, d.Name(), ".dolt")); err == nil && st.IsDir() {
				e.dolt = filepath.Join(base, d.Name())
				return nil
			}
		}
	}
	return fmt.Errorf("sqlitelite: no Dolt database under %s", base)
}

// copyBeads copies <src>/.beads to <dst>/.beads. The busy noms/LOCK file is skipped.
func copyBeads(src, dst string) error {
	from := filepath.Join(src, ".beads")
	to := filepath.Join(dst, ".beads")
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		t := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		if !d.Type().IsRegular() || d.Name() == "LOCK" {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(t)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// Cleanup removes the working copy this extractor made (only its own).
func (e *Extractor) Cleanup() error {
	if e.root == "" {
		return nil
	}
	err := errors.Join(os.RemoveAll(e.root), os.RemoveAll(e.doltR))
	e.root = ""
	return err
}

func (e *Extractor) readExportAndConfig(r *Result) error {
	t := time.Now()
	out, err := e.run("export")
	if err != nil {
		return err
	}
	r.Timing.ExportMs = time.Since(t).Milliseconds()
	r.IssuesJSONL = out
	for _, ln := range bytes.Split(out, []byte("\n")) {
		ln = bytes.TrimSpace(ln)
		if len(ln) == 0 {
			continue
		}
		var h struct {
			Type      string `json:"_type"`
			ID        string `json:"id"`
			UpdatedAt string `json:"updated_at"`
		}
		if err := json.Unmarshal(ln, &h); err != nil {
			return fmt.Errorf("sqlitelite: bad export line: %w", err)
		}
		if h.Type != "issue" || h.ID == "" {
			continue // memories and other record types carry no history
		}
		r.Beads = append(r.Beads, &BeadHistory{ID: h.ID, UpdatedAt: h.UpdatedAt,
			Snapshots: json.RawMessage("[]"), Events: json.RawMessage("[]")})
	}
	t = time.Now()
	cfg, err := e.run("config", "list", "--json")
	if err != nil {
		return err
	}
	r.Timing.ConfigMs = time.Since(t).Milliseconds()
	if err := json.Unmarshal(cfg, &r.Config); err != nil {
		return fmt.Errorf("sqlitelite: bad config json: %w", err)
	}
	return nil
}

// Tables the export/config path (or the loader's own derivation) carries, and
// bookkeeping tables. Every other base table is read and reported.
var coveredTables = map[string]bool{
	"issues": true, "comments": true, "dependencies": true, "labels": true,
	"config": true, "events": true,
	"schema_migrations": true, "ignored_schema_migrations": true,
}

// lite: every column (d.*), not a fixed 18: a version that lacks a column the
// export has never matches the export and gets a spurious final version.
const diffSQL = "SELECT d.*, l.committer " +
	"FROM dolt_diff_issues d JOIN dolt_log l ON l.commit_hash = d.to_commit " +
	"WHERE d.diff_type <> 'removed' ORDER BY d.to_id, l.commit_order"

// readDolt lists the tables, then runs ONE batch with everything else.
func (e *Extractor) readDolt(r *Result) error {
	out, err := e.runDolt("SHOW FULL TABLES WHERE Table_type = 'BASE TABLE';", "sql", "-r", "json")
	if err != nil {
		return err
	}
	docs, err := decodeDocs(out)
	if err != nil || len(docs) != 1 {
		return fmt.Errorf("sqlitelite: table list: %v (%d docs)", err, len(docs))
	}
	var tables []string
	for _, row := range docs[0] {
		for k, v := range row {
			if strings.HasPrefix(k, "Tables_in_") {
				tables = append(tables, fmt.Sprint(v))
			}
		}
	}
	sort.Strings(tables)

	// Statement order: dolt_status count, a count per table, diff, events,
	// then SELECT * for every uncovered table.
	var sb strings.Builder
	sb.WriteString("SELECT COUNT(*) AS n FROM dolt_status;\n")
	for _, t := range tables {
		fmt.Fprintf(&sb, "SELECT COUNT(*) AS n FROM `%s`;\n", t)
	}
	sb.WriteString(diffSQL + ";\n")
	sb.WriteString("SELECT id, issue_id, event_type, actor, old_value, new_value, comment, created_at FROM events ORDER BY issue_id, created_at, id;\n")
	var dump []string
	for _, t := range tables {
		if !coveredTables[t] {
			dump = append(dump, t)
			fmt.Fprintf(&sb, "SELECT * FROM `%s`;\n", t)
		}
	}
	out, err = e.runDolt(sb.String(), "sql", "-r", "json")
	if err != nil {
		return err
	}
	docs, err = decodeDocs(out)
	if err != nil {
		return err
	}
	want := 1 + len(tables) + 2 + len(dump)
	if len(docs) != want {
		return fmt.Errorf("sqlitelite: dolt returned %d result sets, want %d", len(docs), want)
	}
	counts := map[string]int{}
	for i, t := range tables {
		counts[t] = atoi(docs[1+i])
	}
	if n := atoi(docs[0]); n != 0 {
		r.Uncovered["dolt_uncommitted"] = strconv.Itoa(n) // working-set changes would be lost
	}
	diff, events := docs[1+len(tables)], docs[2+len(tables)]
	for i, t := range dump {
		rows := docs[3+len(tables)+i]
		if len(rows) != counts[t] {
			return fmt.Errorf("sqlitelite: table %s: count %d != rows %d", t, counts[t], len(rows))
		}
		r.Uncovered[t] = strconv.Itoa(len(rows))
		if len(rows) > 0 {
			for _, row := range rows {
				for k, v := range row {
					row[k] = plain(v)
				}
			}
			r.Tables[t] = rows
		}
	}
	if counts["events"] != len(events) {
		return fmt.Errorf("sqlitelite: events count %d != rows %d", counts["events"], len(events))
	}
	return e.buildBeads(r, diff, events)
}

func atoi(rows []map[string]any) int {
	if len(rows) != 1 {
		return -1
	}
	n, _ := strconv.Atoi(fmt.Sprint(rows[0]["n"]))
	return n
}

// plain turns json.Number into int64/float64 so the loader can bind it.
func plain(v any) any {
	if n, ok := v.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i
		}
		f, _ := n.Float64()
		return f
	}
	return v
}

// decodeDocs reads the {"rows":[...]} documents `dolt sql -r json` prints.
func decodeDocs(b []byte) ([][]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out [][]map[string]any
	for {
		var d struct {
			Rows []map[string]any `json:"rows"`
		}
		if err := dec.Decode(&d); err == io.EOF {
			return out, nil
		} else if err != nil {
			return nil, fmt.Errorf("sqlitelite: bad dolt json: %w", err)
		}
		out = append(out, d.Rows)
	}
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func parseDolt(v any) time.Time {
	s := str(v)
	if s == "" {
		return time.Time{}
	}
	for _, l := range []string{"2006-01-02 15:04:05.999999999", time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if t, err := time.ParseInLocation(l, s, time.UTC); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func reverse[T any](l []T) {
	for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
		l[i], l[j] = l[j], l[i]
	}
}

func (e *Extractor) buildBeads(r *Result, diff, events []map[string]any) error {
	byID := map[string]*BeadHistory{}
	for _, b := range r.Beads {
		byID[b.ID] = b
	}
	snaps := map[string][]*storage.HistoryEntry{}
	colsByID := map[string][]map[string]any{}
	for _, d := range diff {
		id := str(d["to_id"])
		if byID[id] == nil {
			continue // not exported (wisp, deleted)
		}
		p := str(d["to_pinned"])
		is := &types.Issue{
			ID: id, Title: str(d["to_title"]), Description: str(d["to_description"]),
			Design: str(d["to_design"]), AcceptanceCriteria: str(d["to_acceptance_criteria"]),
			Notes: str(d["to_notes"]), Status: types.Status(str(d["to_status"])),
			IssueType: types.IssueType(str(d["to_issue_type"])), Assignee: str(d["to_assignee"]),
			Owner: str(d["to_owner"]), CreatedBy: str(d["to_created_by"]),
			CreatedAt: parseDolt(d["to_created_at"]), UpdatedAt: parseDolt(d["to_updated_at"]),
			CloseReason: str(d["to_close_reason"]), MolType: types.MolType(str(d["to_mol_type"])),
			Pinned: p != "" && p != "0" && p != "false",
		}
		is.Priority, _ = strconv.Atoi(str(d["to_priority"]))
		if m := str(d["to_estimated_minutes"]); m != "" {
			n, _ := strconv.Atoi(m)
			is.EstimatedMinutes = &n
		}
		if c := parseDolt(d["to_closed_at"]); !c.IsZero() {
			is.ClosedAt = &c
		}
		cm := map[string]any{}
		for k, v := range d {
			if c, ok := strings.CutPrefix(k, "to_"); ok && c != "commit" && c != "commit_date" {
				cm[c] = v
			}
		}
		colsByID[id] = append(colsByID[id], cm)
		snaps[id] = append(snaps[id], &storage.HistoryEntry{
			CommitHash: str(d["to_commit"]), Committer: str(d["committer"]),
			CommitDate: parseDolt(d["to_commit_date"]), Issue: is})
		r.Timing.Versions++
	}
	for id, list := range snaps { // diff is oldest first; upstream prints newest first
		reverse(list)
		b, err := json.Marshal(list)
		if err != nil {
			return err
		}
		byID[id].Snapshots = b
		cl := colsByID[id]
		reverse(cl)
		if byID[id].Cols, err = json.Marshal(cl); err != nil {
			return err
		}
	}
	evs := map[string][]*types.Event{}
	for _, d := range events {
		id := str(d["issue_id"])
		if byID[id] == nil {
			continue
		}
		ev := &types.Event{ID: str(d["id"]), IssueID: id, EventType: types.EventType(str(d["event_type"])),
			Actor: str(d["actor"]), CreatedAt: parseDolt(d["created_at"])}
		for _, f := range []struct {
			k string
			p **string
		}{{"old_value", &ev.OldValue}, {"new_value", &ev.NewValue}, {"comment", &ev.Comment}} {
			if v, ok := d[f.k]; ok && v != nil {
				s := str(v)
				*f.p = &s
			}
		}
		evs[id] = append(evs[id], ev)
		r.Timing.Events++
	}
	for id, list := range evs { // rows are oldest first; upstream prints newest first
		reverse(list)
		b, err := json.Marshal(list)
		if err != nil {
			return err
		}
		byID[id].Events = b
	}
	return nil
}

// Nonzero lists uncovered tables that hold rows (or could not be counted).
func (r *Result) Nonzero() []string {
	var out []string
	for k, v := range r.Uncovered {
		if v != "0" {
			out = append(out, k+"="+v)
		}
	}
	sort.Strings(out)
	return out
}

// oracle compares up to OracleBeads beads (the most versioned half plus a
// seeded random half) against upstream `bd history --json` and `--events
// --json` on the working copy, after collapsing consecutive identical states.
// Any difference is an error.
func (e *Extractor) oracle(r *Result) (int, error) {
	type cand struct {
		b *BeadHistory
		n int
	}
	var cs []cand
	for _, b := range r.Beads {
		var l []json.RawMessage
		_ = json.Unmarshal(b.Snapshots, &l)
		cs = append(cs, cand{b, len(l)})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].n > cs[j].n })
	want := e.o.OracleBeads
	if want > len(cs) {
		want = len(cs)
	}
	top := want / 2
	pick := append([]cand{}, cs[:top]...)
	rest := cs[top:]
	rng := rand.New(rand.NewSource(1))
	rng.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	pick = append(pick, rest[:want-top]...)

	var diffs []string
	for _, c := range pick {
		id := c.b.ID
		up, err := e.run("history", id, "--json")
		if err != nil {
			return 0, err
		}
		if m := compareHistory(c.b.Snapshots, normArray(up)); m != "" {
			diffs = append(diffs, id+" history: "+m)
		}
		ue, err := e.run("history", id, "--events", "--json")
		if err != nil {
			return 0, err
		}
		if m := compareEvents(c.b.Events, normArray(ue)); m != "" {
			diffs = append(diffs, id+" events: "+m)
		}
		e.say("oracle %s ok=%v", id, len(diffs) == 0)
	}
	if len(diffs) > 0 {
		return len(pick), fmt.Errorf("sqlitelite: oracle mismatch on %d of %d beads: %s", len(diffs), len(pick), strings.Join(diffs, "; "))
	}
	return len(pick), nil
}

// normArray turns empty output or JSON null into [].
func normArray(b []byte) json.RawMessage {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || string(t) == "null" {
		return json.RawMessage("[]")
	}
	return json.RawMessage(t)
}

// canon rewrites every RFC 3339 string as UTC at nanosecond precision, so
// equal instants compare equal.
func canon(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, y := range x {
			x[k] = canon(y)
		}
	case []any:
		for i, y := range x {
			x[i] = canon(y)
		}
	case string:
		if len(x) > 18 && x[4] == '-' && x[10] == 'T' {
			if t, err := time.Parse(time.RFC3339Nano, x); err == nil {
				return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
			}
		}
	}
	return v
}

func canonList(raw json.RawMessage) ([]any, error) {
	var l []any
	if err := json.Unmarshal(raw, &l); err != nil {
		return nil, err
	}
	for i := range l {
		l[i] = canon(l[i])
	}
	return l, nil
}

// collapse keeps, oldest to newest, only entries whose Issue differs from the
// previous kept one. Input and output are newest first.
func collapse(l []any) []string {
	var kept []string
	prev := ""
	for i := len(l) - 1; i >= 0; i-- {
		m, _ := l[i].(map[string]any)
		is, _ := json.Marshal(m["Issue"])
		if string(is) == prev {
			continue
		}
		prev = string(is)
		b, _ := json.Marshal(m)
		kept = append(kept, string(b))
	}
	reverse(kept)
	return kept
}

func compareHistory(ours, upstream json.RawMessage) string {
	a, err := canonList(ours)
	if err != nil {
		return "ours: " + err.Error()
	}
	b, err := canonList(upstream)
	if err != nil {
		return "upstream: " + err.Error()
	}
	ca, cb := collapse(a), collapse(b)
	if len(ca) != len(cb) {
		return fmt.Sprintf("%d states vs upstream %d", len(ca), len(cb))
	}
	for i := range ca {
		if ca[i] != cb[i] {
			return fmt.Sprintf("state %d differs: ours %s | bd %s", i, tail(ca[i], 700), tail(cb[i], 700))
		}
	}
	return ""
}

// compareEvents compares as sets: upstream orders only by created_at, ties are undefined.
func compareEvents(ours, upstream json.RawMessage) string {
	a, err := canonList(ours)
	if err != nil {
		return "ours: " + err.Error()
	}
	b, err := canonList(upstream)
	if err != nil {
		return "upstream: " + err.Error()
	}
	enc := func(l []any) []string {
		var s []string
		for _, x := range l {
			j, _ := json.Marshal(x)
			s = append(s, string(j))
		}
		sort.Strings(s)
		return s
	}
	sa, sb := enc(a), enc(b)
	if len(sa) != len(sb) {
		return fmt.Sprintf("%d events vs upstream %d", len(sa), len(sb))
	}
	for i := range sa {
		if sa[i] != sb[i] {
			return "event differs: ours " + tail(sa[i], 300) + " bd " + tail(sb[i], 300)
		}
	}
	return ""
}
