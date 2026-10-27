package sqlitelite

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/steveyegge/beads/internal/storage/depid"
	"github.com/steveyegge/beads/internal/storage/sqlitestore"
	"github.com/steveyegge/beads/internal/types"
)

const tIssue1 = `{"_type":"issue","id":"p-1","title":"T \u0639\u0631\u0628\u064a","description":"line1\nline2  \r\n","design":"d","status":"open","priority":1,"issue_type":"task","assignee":"swe","created_at":"2026-10-25T07:51:45Z","created_by":"cos","updated_at":"2026-10-25T08:00:00Z","labels":["a","b"],"metadata":{"card_q":"why?","title_ar":"x","asked_at":"2026-10-22T00:00:00Z"},"dependencies":[{"issue_id":"p-1","depends_on_id":"p-2","type":"blocks","created_at":"2026-10-25T07:51:45Z","created_by":"cos","metadata":"{}"}],"comments":[{"id":"c1","issue_id":"p-1","author":"cos","text":"hello","created_at":"2026-10-25T07:52:00Z"}],"lease_expires_at":"2026-10-25T09:00:00Z","heartbeat_at":"2026-10-25T08:30:00Z","dependency_count":1,"dependent_count":0,"comment_count":1}`
const tIssue2 = `{"_type":"issue","id":"p-2","title":"two","status":"wip","priority":2,"issue_type":"task","created_at":"2026-10-25T07:51:45Z","updated_at":"2026-10-25T07:51:45Z","dependencies":[{"issue_id":"p-2","depends_on_id":"external:x:y","type":"related","created_at":"2026-10-25T07:51:45Z"}]}`

func testInput() *Input {
	snap := func(title, upd string) map[string]any {
		return map[string]any{"CommitHash": "h", "Committer": "beads", "CommitDate": "2026-10-25T12:17:30.269+03:00",
			"Issue": map[string]any{"id": "p-1", "title": title, "priority": 1, "status": "open", "updated_at": upd, "created_at": "2026-10-25T07:51:45Z"}}
	}
	snaps, _ := json.Marshal([]any{snap("T new", "2026-10-25T08:00:00Z"), snap("T old", "2026-10-25T07:51:45Z")})
	return &Input{
		IssuesJSONL: []byte(tIssue1 + "\n" + tIssue2 + "\n" + `{"_type":"memory","key":"k","value":"v"}` + "\n"),
		Config:      map[string]any{"issue_prefix": "p", "status.custom": "wip:active"},
		Beads: []*BeadHistory{
			{ID: "p-1", Snapshots: snaps, Events: json.RawMessage(`[{"event_type":"created","actor":"cos","created_at":"2026-10-25T07:51:45Z"}]`)},
			{ID: "p-2", Snapshots: json.RawMessage(`[]`), Events: json.RawMessage(`[]`)},
		},
		Tables: map[string][]map[string]any{"local_metadata": {{"key": "k", "value": "v"}}},
	}
}

func TestLoadVerifyRoundtrip(t *testing.T) {
	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "beads.db")
	in := testInput()
	rep, err := Load(ctx, in, dest)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Issues != 2 || rep.Rows["labels"] != 2 || rep.Rows["dependencies"] != 2 || rep.Rows["comments"] != 1 ||
		rep.Rows["events"] != 1 || rep.Rows["leases"] != 1 || rep.Rows["local_metadata"] != 1 || rep.Memories != 1 {
		t.Fatalf("report: %+v", rep)
	}
	// p-1: 2 snapshots, newest differs from export (no description) => appended 3rd.
	if rep.Versions != 4 { // 2 snapshots + 1 appended for p-1, 1 export-only for p-2
		t.Fatalf("versions %d %+v", rep.Versions, rep)
	}
	for _, s := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dest + s); err == nil {
			t.Fatalf("%s left behind", s)
		}
	}
	v, err := Verify(ctx, in, dest, VerifyOptions{Loaded: rep, Tables: in.Tables})
	if err != nil {
		t.Fatal(err)
	}
	if !v.OK || v.ShowDiff != "skipped" {
		t.Fatalf("verify: %+v", v)
	}
	if got := strings.Join(v.Beads[0].MetaKeys, ","); got != "asked_at,card_q,title_ar" {
		t.Fatalf("meta keys %s", got)
	}

	// History reads back through the store, head = current title.
	st, err := sqlitestore.Open(ctx, dest, sqlitestore.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h, err := st.History(ctx, "p-1")
	if err != nil || len(h) != 3 || h[0].Issue.Title != "T \u0639\u0631\u0628\u064a" || h[2].Issue.Title != "T old" {
		t.Fatalf("history: %v %v", err, h)
	}
	if h[1].CommitDate.UTC().Format("15:04") != "09:17" {
		t.Fatalf("commit date not UTC: %v", h[1].CommitDate)
	}
}

func TestVerifyCatchesDifferences(t *testing.T) {
	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "beads.db")
	in := testInput()
	if _, err := Load(ctx, in, dest); err != nil {
		t.Fatal(err)
	}
	// Tamper with a copy of the expectations: description and a label differ.
	bad := *in
	bad.IssuesJSONL = []byte(strings.Replace(strings.Replace(string(in.IssuesJSONL), `"line1\nline2  \r\n"`, `"line1\nline2"`, 1), `"labels":["a","b"]`, `"labels":["a"]`, 1))
	v, err := Verify(ctx, &bad, dest, VerifyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if v.OK {
		t.Fatal("expected differences")
	}
	d := strings.Join(v.Beads[0].Diffs, "|")
	if !strings.Contains(d, "description differs") || !strings.Contains(d, "labels differ") {
		t.Fatalf("diffs: %s", d)
	}
	// Show diff hook: unequal JSON is counted.
	v, _ = Verify(ctx, in, dest, VerifyOptions{Show: func(id string) ([]byte, []byte, error) {
		return []byte(`{"a":1}`), []byte(`{"a":2}`), nil
	}})
	if v.OK || !strings.HasPrefix(v.ShowDiff, "2 differences") {
		t.Fatalf("show: %+v", v)
	}
}

func TestLoadRefusals(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dest := filepath.Join(dir, "beads.db")
	if err := os.WriteFile(dest, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(ctx, testInput(), dest); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing db: %v", err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "x" {
		t.Fatal("existing file touched")
	}
	in := testInput()
	in.Tables["nope"] = []map[string]any{{"a": 1}}
	d2 := filepath.Join(dir, "two.db")
	if _, err := Load(ctx, in, d2); err == nil {
		t.Fatal("unknown table accepted")
	}
	if _, err := os.Stat(d2); err == nil {
		t.Fatal("partial db left behind")
	}
	in = testInput()
	in.Tables = map[string][]map[string]any{"local_metadata": {{"key": "k", "value": "v", "x`;drop": 1}}}
	if _, err := Load(ctx, in, filepath.Join(dir, "three.db")); err == nil {
		t.Fatal("unknown column accepted")
	}
}

func TestCheckUncovered(t *testing.T) {
	r := &Result{Uncovered: map[string]string{"local_metadata": "unreadable: x", "leases": "unreadable: y", "wisps": "0"}}
	if err := CheckUncovered(r, nil); err == nil {
		t.Fatal("must fail closed")
	}
	if err := CheckUncovered(r, []string{"local_metadata", "leases"}); err != nil {
		t.Fatal(err)
	}
}

// lite: a column added after an old commit reads NULL there and 0 in the live
// row; that is not a new version. A real difference is named.
func TestFirstDiffColumnTreatsNullAsZeroDefault(t *testing.T) {
	old := map[string]any{"title": "t", "compaction_level": nil, "metadata": nil, "priority": int64(1)}
	cur := map[string]any{"title": "t", "compaction_level": int64(0), "metadata": "{}", "priority": int64(1)}
	if c := firstDiffColumn(old, cur); c != "" {
		t.Fatalf("zero defaults differ at %q", c)
	}
	cur["priority"] = int64(2)
	if c := firstDiffColumn(old, cur); c != "priority" {
		t.Fatalf("first differing column = %q, want priority", c)
	}
}

func TestSameJSONIgnoresRevisionAndDefaultWorkType(t *testing.T) {
	a := []byte(`[{"id":"x","revision":"-14","dependencies":[{"id":"y"}]}]`)
	b := []byte(`[{"id":"x","revision":"0","work_type":"mutex","dependencies":[{"id":"y","work_type":"mutex"}]}]`)
	if !sameJSON(a, b) {
		t.Fatal("revision and default work_type must not count")
	}
	if sameJSON(a, []byte(`[{"id":"x","work_type":"other","dependencies":[{"id":"y"}]}]`)) {
		t.Fatal("a real work_type difference must count")
	}
}

// Regression (show --json parity, whisper-flow-6bb.9): upstream 1.3.1 lists
// dependencies in primary-key order, and its key is depid.New(issue, target),
// not the loader's own hash. The export carries no ids, so the loader must
// derive the same ids and the fork must read in id order.
func TestDependencyOrderMatchesDoltPrimaryKeyOrder(t *testing.T) {
	ctx := context.Background()
	targets := []string{"q-a", "q-b", "q-c", "q-d", "q-e"} // export order = sorted by target
	var jsonl, deps []string
	for _, id := range targets {
		jsonl = append(jsonl, `{"_type":"issue","id":"`+id+`","title":"t","status":"open","priority":2,"issue_type":"task","created_at":"2026-10-25T07:51:45Z","updated_at":"2026-10-25T07:51:45Z"}`)
		deps = append(deps, `{"issue_id":"q-x","depends_on_id":"`+id+`","type":"blocks","created_at":"2026-10-25T07:51:45Z","created_by":"c","metadata":"{}"}`)
	}
	jsonl = append(jsonl, `{"_type":"issue","id":"q-x","title":"x","status":"open","priority":2,"issue_type":"task","created_at":"2026-10-25T07:51:45Z","updated_at":"2026-10-25T07:51:45Z","dependencies":[`+strings.Join(deps, ",")+`]}`)
	in := &Input{IssuesJSONL: []byte(strings.Join(jsonl, "\n") + "\n"), Config: map[string]any{"issue_prefix": "q"}}
	dest := filepath.Join(t.TempDir(), "beads.db")
	if _, err := Load(ctx, in, dest); err != nil {
		t.Fatal(err)
	}
	st, err := sqlitestore.Open(ctx, dest, sqlitestore.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.GetDependenciesWithMetadata(ctx, "q-x")
	if err != nil || len(got) != len(targets) {
		t.Fatalf("deps: %v %d", err, len(got))
	}
	want := append([]string(nil), targets...)
	sort.Slice(want, func(i, j int) bool { return depid.New("q-x", want[i]) < depid.New("q-x", want[j]) })
	for i, d := range got {
		if d.ID != want[i] {
			t.Fatalf("position %d: got %s want %s (order must follow depid primary key)", i, d.ID, want[i])
		}
	}
	if want[0] == targets[0] && want[1] == targets[1] && want[2] == targets[2] {
		t.Fatal("test data does not exercise ordering; pick other targets")
	}
}

// T045: is_blocked is derived and not loaded from the export; the loader must
// rebuild it, or blocked returns [] and ready lists blocked beads.
func TestLoadRebuildsIsBlocked(t *testing.T) {
	ctx := context.Background()
	dest := filepath.Join(t.TempDir(), "beads.db")
	if _, err := Load(ctx, testInput(), dest); err != nil {
		t.Fatal(err)
	}
	st, err := sqlitestore.Open(ctx, dest, sqlitestore.Options{Actor: "t"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var blocked int
	if err := st.DB().QueryRowContext(ctx, "SELECT is_blocked FROM issues WHERE id='p-1'").Scan(&blocked); err != nil {
		t.Fatal(err)
	}
	if blocked != 1 {
		t.Fatalf("p-1 depends on open p-2 via blocks: is_blocked=%d, want 1", blocked)
	}
	var other int
	if err := st.DB().QueryRowContext(ctx, "SELECT is_blocked FROM issues WHERE id='p-2'").Scan(&other); err != nil || other != 0 {
		t.Fatalf("p-2 is_blocked=%d err=%v, want 0", other, err)
	}
}

// T046 + T049: list lists dependencies in primary-key (depid) order like
// upstream Dolt (export stays sorted by target); the default work_type 'mutex' is not
// reported (upstream omits it).
func TestReadsDependencyOrderAndWorkType(t *testing.T) {
	ctx := context.Background()
	targets := []string{"q-a", "q-b", "q-c", "q-d", "q-e"}
	var lines []string
	var deps []string
	for _, tg := range targets {
		lines = append(lines, `{"_type":"issue","id":"`+tg+`","title":"t","status":"open","priority":2,"issue_type":"task","created_at":"2026-10-25T07:51:45Z","updated_at":"2026-10-25T07:51:45Z"}`)
		deps = append(deps, `{"issue_id":"q-src","depends_on_id":"`+tg+`","type":"blocks","created_at":"2026-10-25T07:51:45Z","created_by":"x","metadata":"{}"}`)
	}
	lines = append(lines, `{"_type":"issue","id":"q-src","title":"s","status":"open","priority":2,"issue_type":"task","created_at":"2026-10-25T07:51:45Z","updated_at":"2026-10-25T07:51:45Z","dependencies":[`+strings.Join(deps, ",")+`]}`)
	in := &Input{IssuesJSONL: []byte(strings.Join(lines, "\n") + "\n"), Config: map[string]any{"issue_prefix": "q"}}
	dest := filepath.Join(t.TempDir(), "beads.db")
	if _, err := Load(ctx, in, dest); err != nil {
		t.Fatal(err)
	}
	st, err := sqlitestore.Open(ctx, dest, sqlitestore.Options{Actor: "t"})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	want := append([]string(nil), targets...)
	sort.Slice(want, func(i, j int) bool { return depid.New("q-src", want[i]) < depid.New("q-src", want[j]) })
	check := func(name string, got, want []string) {
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s order %v, want %v", name, got, want)
		}
	}
	// export / dep reads: sorted by target (upstream export order).
	sorted := append([]string(nil), targets...)
	sort.Strings(sorted)
	all, err := st.GetAllDependencyRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var g1 []string
	for _, d := range all["q-src"] {
		g1 = append(g1, d.DependsOnID)
	}
	check("export", g1, sorted)
	// list: primary-key (depid) order.
	page, err := st.SearchIssuesWithCounts(ctx, "", types.IssueFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var g2 []string
	for _, it := range page {
		if it.Issue.ID == "q-src" {
			for _, d := range it.Issue.Dependencies {
				g2 = append(g2, d.DependsOnID)
			}
		}
	}
	check("list", g2, want)
	is, err := st.GetIssue(ctx, "q-src")
	if err != nil {
		t.Fatal(err)
	}
	if is.WorkType != "" {
		t.Fatalf("work_type %q, want empty (mutex default is not printed)", is.WorkType)
	}
}

// Batched show diff: same per-bead strictness, one call per batch, single-id
// fallback for anything the batch did not return.
func TestVerifyShowBatch(t *testing.T) {
	ctx := context.Background()
	in := testInput()
	dest := filepath.Join(t.TempDir(), "beads.db")
	rep, err := Load(ctx, in, dest)
	if err != nil {
		t.Fatal(err)
	}
	calls, singles := 0, 0
	mk := func(diffID, missID string) VerifyOptions {
		calls, singles = 0, 0
		return VerifyOptions{
			Loaded: rep, Tables: in.Tables, ShowBatchSize: 1000,
			ShowBatch: func(ids []string) (map[string][]byte, map[string][]byte, error) {
				calls++
				om, nm := map[string][]byte{}, map[string][]byte{}
				for _, id := range ids {
					if id == missID {
						continue
					}
					om[id] = []byte(`{"id":"` + id + `","a":1}`)
					nm[id] = om[id]
					if id == diffID {
						nm[id] = []byte(`{"id":"` + id + `","a":2}`)
					}
				}
				return om, nm, nil
			},
			Show: func(id string) ([]byte, []byte, error) {
				singles++
				return []byte(`{"id":"` + id + `"}`), []byte(`{"id":"` + id + `"}`), nil
			},
		}
	}
	v, err := Verify(ctx, in, dest, mk("", ""))
	if err != nil || !v.OK || calls != 1 || singles != 0 || !strings.HasPrefix(v.ShowDiff, "0 differences") {
		t.Fatalf("clean: err=%v calls=%d singles=%d %+v", err, calls, singles, v)
	}
	first := v.Beads[0].ID
	v, _ = Verify(ctx, in, dest, mk(first, ""))
	if v.OK || !strings.HasPrefix(v.ShowDiff, "1 differences") {
		t.Fatalf("a difference inside a batch must fail: %+v", v)
	}
	v, _ = Verify(ctx, in, dest, mk("", first))
	if singles != 1 || !v.OK {
		t.Fatalf("missing id must fall back to single show: singles=%d %+v", singles, v)
	}
}
