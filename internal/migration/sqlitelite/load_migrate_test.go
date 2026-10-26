package sqlitelite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigratePipelineWithStandInBd(t *testing.T) {
	t.Setenv("SQLITELITE_FAKE", "1")
	proj := fakeStore(t)
	marker := filepath.Join(proj, ".beads", "embeddeddolt", "data.bin")
	if err := os.WriteFile(marker, []byte("dolt-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := MigrateOptions{Project: proj, BdBin: os.Args[0], DoltBin: os.Args[0], WorkDir: filepath.Join(t.TempDir(), "w"),
		SkipShow: true,
		// the stand-in dolt returns an incomplete lease row; supply a full one
		Tables: func(*Result) (map[string][]map[string]any, error) {
			return map[string][]map[string]any{"leases": {{"issue_id": "p-1", "holder": "swe", "granted_at": "2026-01-01 00:00:00", "lease_expires_at": "2026-01-01 01:00:00", "heartbeat_at": "2026-01-01 00:00:00"}}}, nil
		}}

	// Failure: a bad extra table stops the run and leaves no beads.db.
	bad := opts
	bad.WorkDir = filepath.Join(t.TempDir(), "w")
	bad.Tables = func(*Result) (map[string][]map[string]any, error) {
		return map[string][]map[string]any{"no_such_table": {{"a": 1}}}, nil
	}
	if _, err := Migrate(context.Background(), bad); err == nil {
		t.Fatal("bad table accepted")
	}
	for _, f := range []string{"beads.db", "beads.db.migrating"} {
		if _, err := os.Stat(filepath.Join(proj, ".beads", f)); err == nil {
			t.Fatalf("%s left behind after failure", f)
		}
	}

	rep, err := Migrate(context.Background(), opts)
	if err != nil {
		t.Fatal(err, rep.Text())
	}
	if !rep.Verify.OK || rep.Load.Issues != 2 || rep.Database == "" {
		t.Fatalf("report: %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(proj, ".beads", "beads.db")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(marker); string(b) != "dolt-bytes" {
		t.Fatal("dolt store touched")
	}
	if !strings.Contains(rep.Text(), "show --json: skipped") {
		t.Fatalf("text: %s", rep.Text())
	}

	// A second run must refuse to overwrite.
	opts.WorkDir = filepath.Join(t.TempDir(), "w")
	if _, err := Migrate(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second run: %v", err)
	}
}
