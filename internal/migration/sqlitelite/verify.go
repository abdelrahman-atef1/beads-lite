package sqlitelite

// W8 verifier (T020). Compares the new database with what was extracted, one
// bead at a time, and reports every difference. Any difference makes the
// migration fail: the caller must leave the live store untouched.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/steveyegge/beads/internal/storage/sqlitestore"
	"github.com/steveyegge/beads/internal/types"
)

// BeadCheck is the per-bead line of the report.
type BeadCheck struct {
	ID       string   `json:"id"`
	Comments [2]int   `json:"comments"` // expected, found
	Deps     [2]int   `json:"deps"`
	Labels   [2]int   `json:"labels"`
	Events   [2]int   `json:"events"`
	Versions [2]int   `json:"versions"` // snapshots expected (+ appended), found
	MetaKeys []string `json:"metadata_keys,omitempty"`
	Diffs    []string `json:"diffs,omitempty"`
}

// VerifyReport is the whole verdict.
type VerifyReport struct {
	Beads    []BeadCheck `json:"beads"`
	Diffs    []string    `json:"diffs,omitempty"` // global differences
	ShowDiff string      `json:"show_diff"`       // "0 differences over N ids" or "skipped"
	OK       bool        `json:"ok"`              // no difference found; ShowDiff says whether show --json was compared
}

// ShowFunc returns the `show --json` bytes of one id from the old and the new
// store. Pluggable: the migrate command runs upstream bd on the Dolt copy and
// the fork on the new database.
type ShowFunc func(id string) (oldJSON, newJSON []byte, err error)

// VerifyOptions tunes Verify.
type VerifyOptions struct {
	ShowWorkers int      // parallel Show calls (default 1)
	Show        ShowFunc // nil: show diff is reported "skipped" and does not count as passed
	// Tables are the extra rows passed to Load; used for the expected counts.
	Tables map[string][]map[string]any
	// Loaded is the loader's report; its appended-version list adjusts the
	// expected version count.
	Loaded *LoadReport
}

// Verify opens dbPath read-only and checks it against in.
func Verify(ctx context.Context, in *Input, dbPath string, o VerifyOptions) (*VerifyReport, error) {
	st, err := sqlitestore.Open(ctx, dbPath, sqlitestore.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer st.Close()
	db := st.DB()
	rep := &VerifyReport{ShowDiff: "skipped"}
	hist := map[string]*BeadHistory{}
	for _, b := range in.Beads {
		hist[b.ID] = b
	}
	extra := map[string]bool{}
	if o.Loaded != nil {
		for _, id := range o.Loaded.ExtraVersion {
			extra[id] = true
		}
	}
	known, err := knownStatuses(ctx, db, in.Config)
	if err != nil {
		return nil, err
	}

	exportIDs := map[string]bool{}
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
			continue
		}
		id := rawString(raw["id"])
		exportIDs[id] = true
		bc, err := verifyBead(ctx, db, id, raw, hist[id], extra[id], known)
		if err != nil {
			return nil, err
		}
		rep.Beads = append(rep.Beads, *bc)
	}

	var n int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM issues").Scan(&n); err != nil {
		return nil, err
	}
	if n != len(exportIDs) {
		rep.Diffs = append(rep.Diffs, fmt.Sprintf("issues: expected %d, found %d", len(exportIDs), n))
	}
	for k, v := range in.Config {
		var got sql.NullString
		err := db.QueryRowContext(ctx, "SELECT value FROM config WHERE `key`=?", k).Scan(&got)
		want := configText(v)
		if v == nil {
			continue
		}
		if err != nil || got.String != want {
			rep.Diffs = append(rep.Diffs, "config "+k+": value differs or missing")
		}
	}
	for t, rows := range o.Tables {
		var c int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+strings.ReplaceAll(t, "`", "")+"`").Scan(&c); err != nil {
			return nil, err
		}
		if c < len(rows) {
			rep.Diffs = append(rep.Diffs, fmt.Sprintf("table %s: expected at least %d rows, found %d", t, len(rows), c))
		}
	}

	if o.Show != nil {
		w := o.ShowWorkers
		if w < 1 {
			w = 1
		}
		msgs := make([]string, len(rep.Beads))
		var wg sync.WaitGroup
		next := make(chan int)
		for i := 0; i < w; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range next {
					id := rep.Beads[j].ID
					a, b, err := o.Show(id)
					if err != nil {
						msgs[j] = "show " + id + ": " + err.Error()
					} else if !sameJSON(a, b) {
						msgs[j] = "show --json differs for " + id
					}
				}
			}()
		}
		for j := range rep.Beads {
			next <- j
		}
		close(next)
		wg.Wait()
		diffs := 0
		for _, m := range msgs {
			if m != "" {
				rep.Diffs = append(rep.Diffs, m)
				diffs++
			}
		}
		rep.ShowDiff = fmt.Sprintf("%d differences over %d ids", diffs, len(rep.Beads))
	}

	rep.OK = len(rep.Diffs) == 0
	for _, b := range rep.Beads {
		if len(b.Diffs) > 0 {
			rep.OK = false
		}
	}
	return rep, nil
}

func configText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func sameJSON(a, b []byte) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(bytes.TrimSpace(a), bytes.TrimSpace(b))
	}
	xa, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return bytes.Equal(xa, yb)
}

// knownStatuses = built-in statuses + custom statuses from the table and from
// the status.custom config value.
func knownStatuses(ctx context.Context, db *sql.DB, cfg map[string]any) (map[string]bool, error) {
	k := map[string]bool{}
	for _, s := range []types.Status{types.StatusOpen, types.StatusInProgress, types.StatusBlocked,
		types.StatusDeferred, types.StatusClosed, types.StatusPinned, types.StatusHooked} {
		k[string(s)] = true
	}
	rows, err := db.QueryContext(ctx, "SELECT name FROM custom_statuses")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		k[n] = true
	}
	rows.Close()
	if v, ok := cfg["status.custom"].(string); ok && v != "" {
		if cs, err := types.ParseCustomStatusConfig(v); err == nil {
			for _, c := range cs {
				k[c.Name] = true
			}
		}
	}
	return k, nil
}

func verifyBead(ctx context.Context, db *sql.DB, id string, raw map[string]json.RawMessage, h *BeadHistory, appended bool, known map[string]bool) (*BeadCheck, error) {
	bc := &BeadCheck{ID: id}
	bad := func(f string, a ...any) { bc.Diffs = append(bc.Diffs, fmt.Sprintf(f, a...)) }

	// Byte-equal text columns.
	var title, desc, design, accept, notes, status, meta sql.NullString
	err := db.QueryRowContext(ctx,
		"SELECT title, description, design, acceptance_criteria, notes, status, metadata FROM issues WHERE id=?", id).
		Scan(&title, &desc, &design, &accept, &notes, &status, &meta)
	if err != nil {
		bad("row missing: %v", err)
		return bc, nil
	}
	for _, c := range []struct {
		name string
		got  sql.NullString
	}{{"title", title}, {"description", desc}, {"design", design}, {"acceptance_criteria", accept}, {"notes", notes}, {"status", status}} {
		if _, present := raw[c.name]; c.name == "status" && !present {
			continue // absent in the export: the column default applies
		}
		if want := rawString(raw[c.name]); c.got.String != want {
			bad("%s differs (%d bytes expected, %d found)", c.name, len(want), len(c.got.String))
		}
	}
	if !known[status.String] {
		bad("status %q is neither built-in nor a known custom status", status.String)
	}

	// Metadata keys and values (card_*, title_ar, asked_at, ...).
	var want map[string]json.RawMessage
	if m, ok := raw["metadata"]; ok && string(m) != "null" {
		if err := json.Unmarshal(m, &want); err != nil {
			bad("export metadata is not an object: %v", err)
		}
	}
	var got map[string]json.RawMessage
	_ = json.Unmarshal([]byte(meta.String), &got)
	for k, v := range want {
		bc.MetaKeys = append(bc.MetaKeys, k)
		g, ok := got[k]
		if !ok {
			bad("metadata key %q missing", k)
		} else if !sameJSON(v, g) {
			bad("metadata key %q differs", k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			bad("metadata key %q not in export", k)
		}
	}
	sort.Strings(bc.MetaKeys)

	// Labels (set equality).
	var wl []string
	if v, ok := raw["labels"]; ok {
		_ = json.Unmarshal(v, &wl)
	}
	gl, err := queryStrings(ctx, db, "SELECT label FROM labels WHERE issue_id=? ORDER BY label", id)
	if err != nil {
		return nil, err
	}
	sort.Strings(wl)
	bc.Labels = [2]int{len(wl), len(gl)}
	if strings.Join(wl, "\x00") != strings.Join(gl, "\x00") {
		bad("labels differ: want %v got %v", wl, gl)
	}

	// Dependencies with type (multiset of type|target).
	var wd []struct {
		DependsOn string `json:"depends_on_id"`
		Type      string `json:"type"`
	}
	if v, ok := raw["dependencies"]; ok {
		_ = json.Unmarshal(v, &wd)
	}
	var wdk []string
	for _, d := range wd {
		wdk = append(wdk, d.Type+"|"+d.DependsOn)
	}
	sort.Strings(wdk)
	gdk, err := queryStrings(ctx, db, "SELECT type||'|'||COALESCE(depends_on_issue_id, depends_on_wisp_id, depends_on_external) FROM dependencies WHERE issue_id=? ORDER BY 1", id)
	if err != nil {
		return nil, err
	}
	bc.Deps = [2]int{len(wdk), len(gdk)}
	if strings.Join(wdk, "\x00") != strings.Join(gdk, "\x00") {
		bad("dependencies differ: want %v got %v", wdk, gdk)
	}

	// Comments: count and byte-equal text, in order.
	var wc []struct {
		Author, Text string
	}
	if v, ok := raw["comments"]; ok {
		_ = json.Unmarshal(v, &wc)
	}
	gc, err := queryStrings(ctx, db, "SELECT author||char(0)||text FROM comments WHERE issue_id=? ORDER BY created_at, id", id)
	if err != nil {
		return nil, err
	}
	bc.Comments = [2]int{len(wc), len(gc)}
	if len(wc) == len(gc) {
		ws := make([]string, len(wc))
		for i, c := range wc {
			ws[i] = c.Author + "\x00" + c.Text
		}
		sort.Strings(ws)
		sort.Strings(gc)
		if strings.Join(ws, "\x01") != strings.Join(gc, "\x01") {
			bad("comment text differs")
		}
	} else {
		bad("comments: expected %d, found %d", len(wc), len(gc))
	}

	// Audit events.
	ne := 0
	if h != nil && len(h.Events) > 0 {
		var evs []json.RawMessage
		_ = json.Unmarshal(h.Events, &evs)
		ne = len(evs)
	}
	var ge int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE issue_id=?", id).Scan(&ge); err != nil {
		return nil, err
	}
	bc.Events = [2]int{ne, ge}
	if ne != ge {
		bad("events: expected %d, found %d", ne, ge)
	}

	// History versions.
	nv := 0
	if h != nil && len(h.Snapshots) > 0 {
		var ss []json.RawMessage
		_ = json.Unmarshal(h.Snapshots, &ss)
		nv = len(ss)
	}
	if appended || nv == 0 {
		nv++
	}
	var gv int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM lite_issue_versions WHERE issue_id=?", id).Scan(&gv); err != nil {
		return nil, err
	}
	bc.Versions = [2]int{nv, gv}
	if nv != gv {
		bad("history versions: expected %d, found %d", nv, gv)
	}
	return bc, nil
}

func queryStrings(ctx context.Context, db *sql.DB, q string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s.String)
	}
	return out, rows.Err()
}
