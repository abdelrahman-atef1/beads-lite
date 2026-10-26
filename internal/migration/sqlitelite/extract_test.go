package sqlitelite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain lets the test binary stand in for upstream bd: when
// SQLITELITE_FAKE_BD is set it prints canned bd output and exits.
func TestMain(m *testing.M) {
	if os.Getenv("SQLITELITE_FAKE_BD") != "" {
		os.Exit(fakeBd(os.Args[1:]))
	}
	os.Exit(m.Run())
}

func fakeBd(args []string) int {
	cmd := strings.Join(args, " ")
	upd := "2026-01-01T00:00:00Z"
	if b, err := os.ReadFile(filepath.Join(os.Getenv("BEADS_DIR"), "v.txt")); err == nil {
		upd = strings.TrimSpace(string(b))
	}
	switch {
	case cmd == "export":
		os.Stdout.WriteString(`{"_type":"issue","id":"p-1","updated_at":"` + upd + `"}` + "\n")
		os.Stdout.WriteString(`{"_type":"issue","id":"p-2","updated_at":"2026-01-01T00:00:00Z"}` + "\n")
		os.Stdout.WriteString(`{"_type":"memory","key":"k"}` + "\n")
	case cmd == "config list --json":
		os.Stdout.WriteString(`{"issue_prefix":"p"}`)
	case strings.HasPrefix(cmd, "history ") && strings.HasSuffix(cmd, "--events --json"):
		os.Stdout.WriteString(`[{"event_type":"created"}]`)
	case strings.HasPrefix(cmd, "history "):
		os.Stdout.WriteString(`null`)
	case strings.HasPrefix(cmd, "provenance log"):
		os.Stdout.WriteString(`[{"kind":"git"}]`)
	case cmd == "events export":
		os.Stdout.WriteString("note: journal disabled\n")
	case strings.HasPrefix(cmd, "list "):
		os.Stdout.WriteString(`[{"id":"p-1"},{"id":"p-2"},{"id":"p-w1"}]`)
	default:
		os.Stderr.WriteString("unexpected: " + cmd)
		return 2
	}
	return 0
}

func fakeStore(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".beads", "embeddeddolt"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExtractAndDelta(t *testing.T) {
	t.Setenv("SQLITELITE_FAKE_BD", "1")
	store := fakeStore(t)
	work := filepath.Join(t.TempDir(), "work")
	e, err := New(Options{BdBin: os.Args[0], StoreCopy: store, WorkDir: work, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.Extract()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Beads) != 2 || r.Beads[0].ID != "p-1" {
		t.Fatalf("beads: %+v", r.Beads)
	}
	if string(r.Beads[0].Snapshots) != "[]" || !strings.Contains(string(r.Beads[0].Events), "created") || r.Beads[0].Provenance != 1 {
		t.Fatalf("bead history not normalised: %+v", r.Beads[0])
	}
	if r.Config["issue_prefix"] != "p" {
		t.Fatalf("config: %v", r.Config)
	}
	if r.Uncovered["wisps"] != "1" || r.Uncovered["bd_events_journal"] != "0" || r.Uncovered["provenance_events"] != "2" {
		t.Fatalf("uncovered: %v", r.Uncovered)
	}
	if nz := r.Nonzero(); len(nz) != 4 { // wisps, provenance, local_metadata, leases
		t.Fatalf("nonzero: %v", nz)
	}
	if r.Timing.BdCalls < 9 {
		t.Fatalf("calls: %d", r.Timing.BdCalls)
	}

	// Delta: p-1's updated_at moves, p-2's does not.
	frozen := fakeStore(t)
	if err := os.WriteFile(filepath.Join(frozen, ".beads", "v.txt"), []byte("2026-02-02T00:00:00Z"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := e.Delta(r, frozen)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Timing.Delta || d.Beads[1] != r.Beads[1] || d.Beads[0] == r.Beads[0] {
		t.Fatalf("delta reused wrong beads")
	}
	// export+config (2), history+events+provenance for p-1 (3), events export+list (2)
	if d.Timing.BdCalls != 7 {
		t.Fatalf("delta calls: %d", d.Timing.BdCalls)
	}

	if err := e.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(work, "w0")); !os.IsNotExist(err) {
		t.Fatalf("workdir not cleaned")
	}
}

func TestNewRejectsNonStore(t *testing.T) {
	if _, err := New(Options{BdBin: "x", StoreCopy: t.TempDir(), WorkDir: t.TempDir()}); err == nil {
		t.Fatal("expected error for folder without .beads")
	}
}

// TestExtractCorpus runs against a real COPY of a store with upstream bd 1.3.1
// and appends timing to a JSON file. Skipped unless the env vars are set:
//
//	BD_LITE_EXTRACT_STORE  copy of the project folder (contains .beads)
//	BD_LITE_UPSTREAM_BD    upstream bd 1.3.1 executable
//	BD_LITE_EXTRACT_WORK   empty scratch folder
//	BD_LITE_EXTRACT_NAME   project label for the result
//	BD_LITE_EXTRACT_BENCH  results file (bench/results/migration-extract.json)
func TestExtractCorpus(t *testing.T) {
	store, bd, work := os.Getenv("BD_LITE_EXTRACT_STORE"), os.Getenv("BD_LITE_UPSTREAM_BD"), os.Getenv("BD_LITE_EXTRACT_WORK")
	if store == "" || bd == "" || work == "" {
		t.Skip("corpus extract not exercised: set BD_LITE_EXTRACT_STORE, BD_LITE_UPSTREAM_BD, BD_LITE_EXTRACT_WORK")
	}
	e, err := New(Options{BdBin: bd, StoreCopy: store, WorkDir: work, Workers: 4,
		Progress: func(s string) { t.Log(s) }})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Cleanup()
	r, err := e.Extract()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v uncovered=%v", r.Timing, r.Uncovered)
	out := os.Getenv("BD_LITE_EXTRACT_BENCH")
	if out == "" {
		return
	}
	all := map[string]any{}
	if b, err := os.ReadFile(out); err == nil {
		_ = json.Unmarshal(b, &all)
	}
	all[os.Getenv("BD_LITE_EXTRACT_NAME")] = map[string]any{"timing": r.Timing, "uncovered": r.Uncovered}
	b, _ := json.MarshalIndent(all, "", "  ")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		t.Fatal(err)
	}
}
