package sqlitelite

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the test binary stand in for upstream bd and for the Dolt
// CLI: SQLITELITE_FAKE_BD / SQLITELITE_FAKE_DOLT print canned output and exit.
func TestMain(m *testing.M) {
	if os.Getenv("SQLITELITE_FAKE") != "" {
		// One binary stands in for both programs: dolt gets `sql ...`, bd anything else.
		if len(os.Args) > 1 && os.Args[1] == "sql" {
			os.Exit(fakeDolt(os.Args[1:]))
		}
		os.Exit(fakeBd(os.Args[1:]))
	}
	os.Exit(m.Run())
}

const fakeIssue = `{"Issue":{"id":"p-1","title":"T","status":"open","priority":2,"issue_type":"task","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}}`

func fakeBd(args []string) int {
	cmd := strings.Join(args, " ")
	switch {
	case cmd == "export":
		os.Stdout.WriteString(`{"_type":"issue","id":"p-1","updated_at":"2026-01-01T00:00:00Z"}` + "\n")
		os.Stdout.WriteString(`{"_type":"issue","id":"p-2","updated_at":"2026-01-01T00:00:00Z"}` + "\n")
		os.Stdout.WriteString(`{"_type":"memory","key":"k"}` + "\n")
	case cmd == "config list --json":
		os.Stdout.WriteString(`{"issue_prefix":"p"}`)
	case cmd == "history p-1 --json":
		// two commits, same state, offset time zone: collapses to the oldest
		os.Stdout.WriteString(`[{"CommitHash":"c2","Committer":"beads","CommitDate":"2026-01-02T03:00:00+03:00",` + fakeIssue[1:len(fakeIssue)-1] + `},` +
			`{"CommitHash":"c1","Committer":"beads","CommitDate":"2026-01-01T03:00:00+03:00",` + fakeIssue[1:len(fakeIssue)-1] + `}]`)
	case cmd == "history p-1 --events --json":
		os.Stdout.WriteString(`[{"id":"e1","issue_id":"p-1","event_type":"created","actor":"a","created_at":"2026-01-01T00:00:00Z"}]`)
	case strings.HasPrefix(cmd, "history "):
		os.Stdout.WriteString(`null`)
	default:
		os.Stderr.WriteString("unexpected: " + cmd)
		return 2
	}
	return 0
}

func fakeDolt(args []string) int {
	if os.Getenv("DOLT_ROOT_PATH") == "" {
		os.Stderr.WriteString("no DOLT_ROOT_PATH")
		return 3
	}
	cfg, err := os.ReadFile(filepath.Join(os.Getenv("DOLT_ROOT_PATH"), ".dolt", "config_global.json"))
	if err != nil || !strings.Contains(string(cfg), `"metrics.disabled":"true"`) {
		os.Stderr.WriteString("telemetry not disabled")
		return 3
	}
	in, _ := io.ReadAll(os.Stdin)
	for _, s := range strings.Split(string(in), ";") {
		s = strings.TrimSpace(s)
		switch {
		case s == "":
		case strings.HasPrefix(s, "SHOW FULL TABLES"):
			os.Stdout.WriteString(`{"rows":[{"Tables_in_db":"events","Table_type":"BASE TABLE"},{"Tables_in_db":"issues","Table_type":"BASE TABLE"},{"Tables_in_db":"leases","Table_type":"BASE TABLE"},{"Tables_in_db":"wisps","Table_type":"BASE TABLE"}]}` + "\n")
		case strings.Contains(s, "COUNT(*) AS n FROM `events`"):
			os.Stdout.WriteString(`{"rows":[{"n":1}]}` + "\n")
		case strings.Contains(s, "COUNT(*) AS n FROM `leases`"):
			os.Stdout.WriteString(`{"rows":[{"n":1}]}` + "\n")
		case strings.Contains(s, "COUNT(*) AS n"):
			os.Stdout.WriteString(`{"rows":[{"n":0}]}` + "\n")
		case strings.HasPrefix(s, "SELECT d.to_id"):
			base := `"to_title":"T","to_description":"","to_design":"","to_acceptance_criteria":"","to_notes":"","to_status":"open","to_priority":2,"to_issue_type":"task","to_assignee":null,"to_owner":null,"to_created_by":null,"to_created_at":"2026-01-01 00:00:00","to_updated_at":"2026-01-01 00:00:00","to_pinned":0,"to_close_reason":"","to_mol_type":""`
			os.Stdout.WriteString(`{"rows":[{"to_id":"p-1","to_commit":"c1","to_commit_date":"2026-01-01 00:00:00.000","committer":"beads",` + base + `},` +
				`{"to_id":"p-1","to_commit":"c2","to_commit_date":"2026-01-02 00:00:00.000","committer":"beads",` + base + `},` +
				`{"to_id":"gone","to_commit":"c1","to_commit_date":"2026-01-01 00:00:00.000","committer":"beads",` + base + `}]}` + "\n")
		case strings.HasPrefix(s, "SELECT id, issue_id"):
			os.Stdout.WriteString(`{"rows":[{"id":"e1","issue_id":"p-1","event_type":"created","actor":"a","old_value":null,"new_value":null,"created_at":"2026-01-01 00:00:00"}]}` + "\n")
		case strings.HasPrefix(s, "SELECT * FROM `leases`"):
			os.Stdout.WriteString(`{"rows":[{"issue_id":"p-1","holder":"swe","granted_at":"2026-01-01 00:00:00"}]}` + "\n")
		case strings.HasPrefix(s, "SELECT * FROM `wisps`"):
			os.Stdout.WriteString(`{"rows":[]}` + "\n")
		default:
			os.Stderr.WriteString("unexpected sql: " + s)
			return 2
		}
	}
	return 0
}

// fakeBoth makes one binary stand in for both programs: the env var that
// matters is chosen by the program name argument shape (bd gets args like
// `export`, dolt gets `sql`); the wrapper below picks by first arg.
func TestExtractWithFakes(t *testing.T) {
	store := fakeStore(t)
	work := filepath.Join(t.TempDir(), "work")
	// Same test binary acts as bd or dolt depending on its first argument.
	t.Setenv("SQLITELITE_FAKE", "1")
	e, err := New(Options{BdBin: os.Args[0], DoltBin: os.Args[0], StoreCopy: store, WorkDir: work})
	if err != nil {
		t.Fatal(err)
	}
	e.o.OracleBeads = 2
	r, err := e.Extract()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Beads) != 2 || r.Beads[0].ID != "p-1" {
		t.Fatalf("beads: %+v", r.Beads)
	}
	var snaps []map[string]any
	if err := json.Unmarshal(r.Beads[0].Snapshots, &snaps); err != nil || len(snaps) != 2 || snaps[0]["CommitHash"] != "c2" {
		t.Fatalf("snapshots newest first, two entries: %s (%v)", r.Beads[0].Snapshots, err)
	}
	if string(r.Beads[1].Snapshots) != "[]" {
		t.Fatalf("p-2 snapshots: %s", r.Beads[1].Snapshots)
	}
	if !strings.Contains(string(r.Beads[0].Events), `"created"`) {
		t.Fatalf("events: %s", r.Beads[0].Events)
	}
	if r.Uncovered["leases"] != "1" || r.Uncovered["wisps"] != "0" || len(r.Tables["leases"]) != 1 || r.Tables["wisps"] != nil {
		t.Fatalf("uncovered %v tables %v", r.Uncovered, r.Tables)
	}
	if nz := r.Nonzero(); len(nz) != 1 || nz[0] != "leases=1" {
		t.Fatalf("nonzero: %v", nz)
	}
	if r.Timing.Versions != 2 || r.Timing.Events != 1 || r.Timing.Oracle != 2 || r.Timing.BdCalls != 2+4 {
		t.Fatalf("timing: %+v", r.Timing)
	}
	if err := e.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, "copy")); !os.IsNotExist(err) {
		t.Fatalf("workdir not cleaned")
	}
}

func TestCollapseAndCompare(t *testing.T) {
	a := json.RawMessage(`[{"CommitHash":"2","Issue":{"t":"x"}},{"CommitHash":"1","Issue":{"t":"x"}}]`)
	b := json.RawMessage(`[{"CommitHash":"1","Issue":{"t":"x"}}]`)
	if m := compareHistory(a, b); m != "" {
		t.Fatalf("expected equal after collapse: %s", m)
	}
	c := json.RawMessage(`[{"CommitHash":"1","Issue":{"t":"y"}}]`)
	if m := compareHistory(a, c); m == "" {
		t.Fatal("expected difference")
	}
	// same instant, different zone
	d := json.RawMessage(`[{"CommitDate":"2026-01-01T03:00:00+03:00"}]`)
	e := json.RawMessage(`[{"CommitDate":"2026-01-01T00:00:00Z"}]`)
	if m := compareHistory(d, e); m != "" {
		t.Fatalf("zone should not matter: %s", m)
	}
}

func fakeStore(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".beads", "embeddeddolt", "db", ".dolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestNewRejectsNonStore(t *testing.T) {
	if _, err := New(Options{BdBin: "x", StoreCopy: t.TempDir(), WorkDir: t.TempDir()}); err == nil {
		t.Fatal("expected error for folder without .beads")
	}
}

// TestExtractCorpus runs against a real COPY of a store with upstream bd 1.3.1
// and the pinned Dolt CLI, and records timing in a JSON file. Skipped unless
// the env vars are set:
//
//	BD_LITE_EXTRACT_STORE  copy of the project folder (contains .beads)
//	BD_LITE_UPSTREAM_BD    upstream bd 1.3.1 executable
//	BD_LITE_EXTRACT_WORK   empty scratch folder
//	BD_LITE_EXTRACT_NAME   project label for the result
//	BD_LITE_EXTRACT_BENCH  results file (bench/results/migration-extract.json)
//	BD_LITE_DOLT           optional Dolt CLI path
func TestExtractCorpus(t *testing.T) {
	store, bd, work := os.Getenv("BD_LITE_EXTRACT_STORE"), os.Getenv("BD_LITE_UPSTREAM_BD"), os.Getenv("BD_LITE_EXTRACT_WORK")
	if store == "" || bd == "" || work == "" {
		t.Skip("corpus extract not exercised: set BD_LITE_EXTRACT_STORE, BD_LITE_UPSTREAM_BD, BD_LITE_EXTRACT_WORK")
	}
	e, err := New(Options{BdBin: bd, StoreCopy: store, WorkDir: work, Progress: func(s string) { t.Log(s) }})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Cleanup()
	r, runErr := e.Extract()
	if r != nil {
		t.Logf("%+v uncovered=%v", r.Timing, r.Uncovered)
		if out := os.Getenv("BD_LITE_EXTRACT_BENCH"); out != "" {
			all := map[string]any{}
			if b, err := os.ReadFile(out); err == nil {
				_ = json.Unmarshal(b, &all)
			}
			entry := map[string]any{"timing": r.Timing, "uncovered": r.Uncovered, "oracle_equal": runErr == nil}
			all[os.Getenv("BD_LITE_EXTRACT_NAME")] = entry
			b, _ := json.MarshalIndent(all, "", "  ")
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(out, b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if runErr != nil {
		t.Fatal(runErr)
	}
}
