package sqlitelite

// Migrate runs the whole W8 pipeline for one project: extract (through
// upstream bd on copies), load into a staging file, verify, and only then move
// the staging file to .beads/beads.db. embeddeddolt/ is never written; on any
// failure the staging file is removed and the project is exactly as it was.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// MigrateOptions configures Migrate.
type MigrateOptions struct {
	// Project is the folder that contains .beads (the frozen store). It is
	// only read: the extractor copies it.
	Project string
	// BdBin is upstream bd 1.3.1.
	BdBin string
	// ForkBin is this fork's executable, used for the new side of the
	// show --json diff. Empty with SkipShow false is an error.
	ForkBin string
	// WorkDir holds the extractor's copies; created fresh, removed after.
	WorkDir string
	// Workers is the number of parallel upstream bd copies for the show diff (default 4).
	Workers int
	// DoltBin overrides the pinned Dolt CLI; OracleBeads the extractor oracle size.
	DoltBin     string
	OracleBeads int
	// Tables supplies the rows the extractor cannot read (the pluggable seam:
	// local_metadata, leases, journal, custom_statuses, ...). Called after
	// extraction; its keys count as covered.
	Tables func(*Result) (map[string][]map[string]any, error)
	// Accept names uncovered tables the caller accepts as empty or lost.
	// Recorded in the report.
	Accept []string
	// SkipShow skips the show --json diff (development only; the report says so).
	SkipShow bool
	Progress func(string)
}

// MigrateReport is what the command prints and saves.
type MigrateReport struct {
	Project   string            `json:"project"`
	Database  string            `json:"database,omitempty"`
	Extract   Timing            `json:"extract"`
	Uncovered map[string]string `json:"uncovered"`
	Accepted  []string          `json:"accepted_uncovered,omitempty"`
	Load      *LoadReport       `json:"load,omitempty"`
	Verify    *VerifyReport     `json:"verify,omitempty"`
	Notes     []string          `json:"notes,omitempty"`
	TotalMs   int64             `json:"total_ms"`
}

// Text is the short human summary.
func (m *MigrateReport) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "migrate %s: %d issues", m.Project, m.Extract.Issues)
	if m.Load != nil {
		fmt.Fprintf(&b, ", %d versions", m.Load.Versions)
		for _, t := range []string{"comments", "dependencies", "labels", "events"} {
			fmt.Fprintf(&b, ", %d %s", m.Load.Rows[t], t)
		}
	}
	if m.Verify != nil {
		fmt.Fprintf(&b, "\nverify: ok=%v, show --json: %s", m.Verify.OK, m.Verify.ShowDiff)
		n := 0
		for _, d := range m.Verify.Diffs {
			if n++; n <= 20 {
				fmt.Fprintf(&b, "\n  %s", d)
			}
		}
		for _, bc := range m.Verify.Beads {
			for _, d := range bc.Diffs {
				if n++; n <= 20 {
					fmt.Fprintf(&b, "\n  %s: %s", bc.ID, d)
				}
			}
		}
		if n > 20 {
			fmt.Fprintf(&b, "\n  ... %d more", n-20)
		}
	}
	for _, n := range m.Notes {
		fmt.Fprintf(&b, "\nnote: %s", n)
	}
	if len(m.Accepted) > 0 {
		fmt.Fprintf(&b, "\naccepted uncovered: %s", strings.Join(m.Accepted, ", "))
	}
	return b.String()
}

// Migrate does the work. The returned report is non-nil whenever extraction
// got far enough to have one, also on error.
func Migrate(ctx context.Context, o MigrateOptions) (*MigrateReport, error) {
	t0 := time.Now()
	beadsDir := filepath.Join(o.Project, ".beads")
	dest := filepath.Join(beadsDir, "beads.db")
	staging := dest + ".migrating"
	if _, err := os.Stat(dest); err == nil {
		return nil, fmt.Errorf("sqlitelite: %s already exists; refusing to overwrite", dest)
	}
	if o.WorkDir == "" {
		o.WorkDir = filepath.Join(os.TempDir(), fmt.Sprintf("lite-migrate-%d", os.Getpid()))
	}
	e, err := New(Options{BdBin: o.BdBin, StoreCopy: o.Project, WorkDir: o.WorkDir, DoltBin: o.DoltBin, OracleBeads: o.OracleBeads, Progress: o.Progress})
	if err != nil {
		return nil, err
	}
	res, err := e.Extract()
	defer e.Cleanup()
	if err != nil {
		return nil, err
	}
	rep := &MigrateReport{Project: o.Project, Extract: res.Timing, Uncovered: res.Uncovered, Accepted: o.Accept, Load: nil}
	in := InputFromResult(res)
	covered := append([]string(nil), o.Accept...)
	for k := range res.Tables { // rows the extractor read itself
		covered = append(covered, k)
	}
	if o.Tables != nil {
		tabs, err := o.Tables(res)
		if err != nil {
			return rep, err
		}
		for k, v := range tabs {
			in.Tables[k] = v
			covered = append(covered, k)
		}
	}
	if err := CheckUncovered(res, covered); err != nil {
		return rep, err
	}

	_ = os.Remove(staging) // a leftover of our own failed run
	cleanup := func() {
		for _, s := range []string{"", "-wal", "-shm", "-journal"} {
			_ = os.Remove(staging + s)
		}
	}
	lr, err := Load(ctx, in, staging)
	if err != nil {
		return rep, err
	}
	rep.Load = lr

	vo := VerifyOptions{Loaded: lr, Tables: in.Tables}
	if !o.SkipShow {
		if o.ForkBin == "" {
			cleanup()
			return rep, errors.New("sqlitelite: ForkBin is required for the show --json diff (or SkipShow)")
		}
		workers := o.Workers
		if workers < 1 {
			workers = 4
		}
		show, done, err := e.showFunc(o.ForkBin, beadsDir, staging, workers)
		if err != nil {
			cleanup()
			return rep, err
		}
		defer done()
		vo.Show, vo.ShowWorkers = show, workers
	}
	vr, err := Verify(ctx, in, staging, vo)
	if err != nil {
		cleanup()
		return rep, err
	}
	rep.Verify = vr
	rep.TotalMs = time.Since(t0).Milliseconds()
	if !vr.OK {
		cleanup()
		return rep, errors.New("sqlitelite: verification found differences; no beads.db was created and the project is unchanged")
	}
	if err := os.Rename(staging, dest); err != nil {
		cleanup()
		return rep, err
	}
	rep.Database = dest
	return rep, nil
}

// showFunc builds the pluggable show diff: old side = upstream bd on one of
// the extractor's copies, new side = the fork on a scratch project holding a
// copy of the staging database and a metadata.json with backend sqlite.
func (e *Extractor) showFunc(forkBin, beadsDir, staging string, workers int) (ShowFunc, func(), error) {
	stage := filepath.Join(e.o.WorkDir+"-newside", "p")
	if err := os.MkdirAll(filepath.Join(stage, ".beads"), 0o755); err != nil {
		return nil, nil, err
	}
	done := func() { _ = os.RemoveAll(e.o.WorkDir + "-newside") }
	if err := copyFile(staging, filepath.Join(stage, ".beads", "beads.db")); err != nil {
		done()
		return nil, nil, err
	}
	meta := map[string]any{}
	if b, err := os.ReadFile(filepath.Join(beadsDir, "metadata.json")); err == nil {
		_ = json.Unmarshal(b, &meta)
	}
	meta["backend"] = "sqlite"
	mb, _ := json.MarshalIndent(meta, "", "  ")
	if err := os.WriteFile(filepath.Join(stage, ".beads", "metadata.json"), mb, 0o644); err != nil {
		done()
		return nil, nil, err
	}
	// Embedded Dolt serialises per folder, so give each worker its own copy.
	pool := make(chan string, workers)
	for i := 0; i < workers; i++ {
		c := filepath.Join(e.o.WorkDir+"-show", fmt.Sprintf("w%d", i))
		if err := copyBeads(e.root, c); err != nil {
			done()
			return nil, nil, err
		}
		pool <- c
	}
	prev := done
	done = func() { prev(); _ = os.RemoveAll(e.o.WorkDir + "-show") }
	return func(id string) ([]byte, []byte, error) {
		root := <-pool
		defer func() { pool <- root }()
		oldOut, err := retryRun(showAttempts, 2*time.Second, func() ([]byte, error) { return runExe(e.o.BdBin, root, "show", id, "--json") })
		if err != nil {
			return nil, nil, err
		}
		newOut, err := retryRun(showAttempts, 2*time.Second, func() ([]byte, error) { return runExe(forkBin, stage, "show", id, "--json") })
		if err != nil {
			return nil, nil, err
		}
		return oldOut, newOut, nil
	}, done, nil
}

func runExe(bin, root string, args ...string) ([]byte, error) {
	c := exec.Command(bin, args...)
	c.Dir = root
	c.Env = append(os.Environ(), "BEADS_DIR="+filepath.Join(root, ".beads"), "BD_DISABLE_METRICS=1", "NO_COLOR=1", "BEADS_ACTOR=lite-verify", "BD_ACTOR=lite-verify")
	hideWindow(c)
	out, err := c.Output()
	if err != nil {
		var ee *exec.ExitError
		msg := err.Error()
		if errors.As(err, &ee) {
			msg += ": " + strings.TrimSpace(tail(string(ee.Stderr), 400))
		}
		return nil, fmt.Errorf("%s %s: %s", filepath.Base(bin), strings.Join(args, " "), msg)
	}
	return out, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// showAttempts is how many times one `show` child may be started before its
// failure counts. A Go runtime crash in the child (exit 2, seen once on fatura
// under memory pressure, T048) must not fail a whole migration; a real
// difference or a bead that does not exist fails identically every time.
const showAttempts = 3

// retryRun calls f up to attempts times, pausing between tries, and returns
// the first success or the last error.
func retryRun(attempts int, pause time.Duration, f func() ([]byte, error)) ([]byte, error) {
	var out []byte
	var err error
	for i := 0; i < attempts; i++ {
		if out, err = f(); err == nil {
			return out, nil
		}
		if i < attempts-1 {
			time.Sleep(pause * time.Duration(i+1))
		}
	}
	return nil, err
}
