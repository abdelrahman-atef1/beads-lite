// Package sqlitelite moves a Dolt-backed beads store into the SQLite store.
//
// This file is the W8 extractor (research R5). It reads the Dolt store ONLY
// through upstream bd 1.3.1 run as a subprocess (export, config list,
// history, provenance, events, list); the fork has no CGO and cannot open
// embedded Dolt, and `bd sql` is refused in embedded mode. It never touches
// the source: the caller passes a COPY of the project folder, and the
// extractor makes further per-worker copies of that copy so the embedded
// engine's lock does not serialize the per-bead history calls.
//
// Spec deviation: `bd export --events` does not exist in bd 1.3.1. Audit
// events come from `bd history <id> --events --json` per bead instead.
package sqlitelite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Options configures an extraction.
type Options struct {
	// BdBin is the upstream bd 1.3.1 executable (never the fork).
	BdBin string
	// StoreCopy is a COPY of the project folder (the folder that contains
	// .beads). It is only read from.
	StoreCopy string
	// WorkDir holds the per-worker copies. Must be empty or absent.
	WorkDir string
	// Workers is the number of parallel copies (default 4).
	Workers int
	// CallTimeout bounds one bd call (default 10 min).
	CallTimeout time.Duration
	// SkipProvenance skips the per-bead provenance count (one bd call per bead).
	SkipProvenance bool
	// Progress, if set, is called with a short line now and then.
	Progress func(string)
}

// BeadHistory is what was read for one bead.
type BeadHistory struct {
	ID         string
	UpdatedAt  string
	Snapshots  json.RawMessage // `bd history <id> --json`: [{CommitHash,Committer,CommitDate,Issue}]
	Events     json.RawMessage // `bd history <id> --events --json`: audit events
	Provenance int             // rows from `bd provenance log`
}

// Result is the full extraction.
type Result struct {
	IssuesJSONL []byte            // `bd export` output, byte for byte
	Config      map[string]any    // `bd config list --json`
	Beads       []*BeadHistory    // one per exported issue, export order
	Uncovered   map[string]string // table -> row count, or "unreadable: why"
	Timing      Timing
}

// Timing records wall time per stage, in milliseconds.
type Timing struct {
	Issues      int   `json:"issues"`
	CopyMs      int64 `json:"copy_ms"`
	ExportMs    int64 `json:"export_ms"`
	ConfigMs    int64 `json:"config_ms"`
	HistoryMs   int64 `json:"history_ms"`
	UncoveredMs int64 `json:"uncovered_ms"`
	TotalMs     int64 `json:"total_ms"`
	Workers     int   `json:"workers"`
	BdCalls     int   `json:"bd_calls"`
	Delta       bool  `json:"delta"`
}

// Extractor runs the extraction. Create with New, call Extract, then
// optionally Delta at freeze time, then Cleanup.
type Extractor struct {
	o       Options
	copies  []string // worker roots
	calls   int
	callsMu sync.Mutex
}

// New validates options.
func New(o Options) (*Extractor, error) {
	if o.BdBin == "" || o.StoreCopy == "" || o.WorkDir == "" {
		return nil, errors.New("sqlitelite: BdBin, StoreCopy and WorkDir are required")
	}
	if st, err := os.Stat(filepath.Join(o.StoreCopy, ".beads")); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("sqlitelite: %s has no .beads folder", o.StoreCopy)
	}
	if o.Workers <= 0 {
		o.Workers = 4
	}
	if o.CallTimeout <= 0 {
		o.CallTimeout = 10 * time.Minute
	}
	return &Extractor{o: o}, nil
}

func (e *Extractor) say(f string, a ...any) {
	if e.o.Progress != nil {
		e.o.Progress(fmt.Sprintf(f, a...))
	}
}

// run executes bd against one root. Returns stdout; stderr goes into the error.
func (e *Extractor) run(root string, args ...string) ([]byte, error) {
	e.callsMu.Lock()
	e.calls++
	e.callsMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), e.o.CallTimeout)
	defer cancel()
	c := exec.CommandContext(ctx, e.o.BdBin, args...)
	c.Dir = root
	c.Env = append(os.Environ(),
		"BEADS_DIR="+filepath.Join(root, ".beads"),
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

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// Extract does the full pre-freeze read.
func (e *Extractor) Extract() (*Result, error) {
	t0 := time.Now()
	r := &Result{Uncovered: map[string]string{}}

	if err := e.makeCopies(); err != nil {
		return nil, err
	}
	r.Timing.CopyMs = time.Since(t0).Milliseconds()

	if err := e.readExportAndConfig(r); err != nil {
		return nil, err
	}

	th := time.Now()
	if err := e.readHistories(r.Beads); err != nil {
		return nil, err
	}
	r.Timing.HistoryMs = time.Since(th).Milliseconds()

	tu := time.Now()
	e.countUncovered(r)
	r.Timing.UncoveredMs = time.Since(tu).Milliseconds()

	r.Timing.Issues = len(r.Beads)
	r.Timing.Workers = len(e.copies)
	r.Timing.BdCalls = e.calls
	r.Timing.TotalMs = time.Since(t0).Milliseconds()
	return r, nil
}

// Delta re-reads, at freeze time, from a copy of the now-frozen store: it
// exports again and re-reads history only for beads that are new or whose
// updated_at moved since prev. Beads that disappeared are dropped. Unchanged
// beads keep their prev history. frozenCopy is a COPY of the frozen folder.
func (e *Extractor) Delta(prev *Result, frozenCopy string) (*Result, error) {
	t0 := time.Now()
	fe, err := New(Options{BdBin: e.o.BdBin, StoreCopy: frozenCopy, WorkDir: e.o.WorkDir + "-delta",
		Workers: e.o.Workers, CallTimeout: e.o.CallTimeout, SkipProvenance: e.o.SkipProvenance, Progress: e.o.Progress})
	if err != nil {
		return nil, err
	}
	if err := fe.makeCopies(); err != nil {
		return nil, err
	}
	defer fe.Cleanup()

	r := &Result{Uncovered: map[string]string{}}
	if err := fe.readExportAndConfig(r); err != nil {
		return nil, err
	}
	old := map[string]*BeadHistory{}
	for _, b := range prev.Beads {
		old[b.ID] = b
	}
	var changed []*BeadHistory
	for i, b := range r.Beads {
		if o, ok := old[b.ID]; ok && o.UpdatedAt == b.UpdatedAt {
			r.Beads[i] = o
			continue
		}
		changed = append(changed, b)
	}
	e.say("delta: %d of %d beads changed", len(changed), len(r.Beads))
	if err := fe.readHistories(changed); err != nil {
		return nil, err
	}
	fe.countUncovered(r)
	r.Timing = Timing{Issues: len(r.Beads), Workers: len(fe.copies), BdCalls: fe.calls, Delta: true,
		TotalMs: time.Since(t0).Milliseconds()}
	return r, nil
}

// makeCopies creates one copy of the store per worker.
func (e *Extractor) makeCopies() error {
	if ents, err := os.ReadDir(e.o.WorkDir); err == nil && len(ents) > 0 {
		return fmt.Errorf("sqlitelite: WorkDir %s is not empty", e.o.WorkDir)
	}
	var wg sync.WaitGroup
	errs := make([]error, e.o.Workers)
	for i := 0; i < e.o.Workers; i++ {
		dst := filepath.Join(e.o.WorkDir, fmt.Sprintf("w%d", i))
		e.copies = append(e.copies, dst)
		wg.Add(1)
		go func(i int, dst string) {
			defer wg.Done()
			errs[i] = copyBeads(e.o.StoreCopy, dst)
		}(i, dst)
	}
	wg.Wait()
	return errors.Join(errs...)
}

// copyBeads copies <src>/.beads to <dst>/.beads.
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
		if !d.Type().IsRegular() {
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

// Cleanup removes the per-worker copies this extractor made (only its own).
func (e *Extractor) Cleanup() error {
	var errs []error
	for _, c := range e.copies {
		errs = append(errs, os.RemoveAll(c))
	}
	e.copies = nil
	return errors.Join(errs...)
}

func (e *Extractor) readExportAndConfig(r *Result) error {
	root := e.copies[0]
	t := time.Now()
	out, err := e.run(root, "export")
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
		r.Beads = append(r.Beads, &BeadHistory{ID: h.ID, UpdatedAt: h.UpdatedAt})
	}
	t = time.Now()
	cfg, err := e.run(root, "config", "list", "--json")
	if err != nil {
		return err
	}
	r.Timing.ConfigMs = time.Since(t).Milliseconds()
	if err := json.Unmarshal(cfg, &r.Config); err != nil {
		return fmt.Errorf("sqlitelite: bad config json: %w", err)
	}
	return nil
}

// readHistories fills Snapshots/Events/Provenance for the given beads, one
// goroutine per worker copy pulling from a shared queue.
func (e *Extractor) readHistories(beads []*BeadHistory) error {
	if len(beads) == 0 {
		return nil
	}
	q := make(chan *BeadHistory)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	done := 0
	for _, root := range e.copies {
		wg.Add(1)
		go func(root string) {
			defer wg.Done()
			for b := range q {
				mu.Lock()
				failed := firstErr != nil
				mu.Unlock()
				if failed {
					continue
				}
				err := e.readOne(root, b)
				mu.Lock()
				if err != nil {
					if firstErr == nil {
						firstErr = err
					}
				} else {
					done++
					if done%200 == 0 {
						e.say("history: %d/%d", done, len(beads))
					}
				}
				mu.Unlock()
			}
		}(root)
	}
	for _, b := range beads {
		q <- b
	}
	close(q)
	wg.Wait()
	return firstErr
}

func (e *Extractor) readOne(root string, b *BeadHistory) error {
	snap, err := e.run(root, "history", b.ID, "--json")
	if err != nil {
		return err
	}
	ev, err := e.run(root, "history", b.ID, "--events", "--json")
	if err != nil {
		return err
	}
	b.Snapshots, b.Events = normArray(snap), normArray(ev)
	if !e.o.SkipProvenance {
		p, err := e.run(root, "provenance", "log", b.ID, "--json")
		if err != nil {
			return err
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(normArray(p), &rows); err != nil {
			return fmt.Errorf("sqlitelite: provenance %s: %w", b.ID, err)
		}
		b.Provenance = len(rows)
	}
	return nil
}

// normArray turns empty output or JSON null into [].
func normArray(b []byte) json.RawMessage {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || string(t) == "null" {
		return json.RawMessage("[]")
	}
	return json.RawMessage(t)
}

// countUncovered fills Uncovered with counts for the tables the export does
// not carry. A table with no supported reader says so; the loader (T020)
// must stop that project's switch on any entry from Nonzero.
func (e *Extractor) countUncovered(r *Result) {
	root := e.copies[0]
	r.Uncovered["local_metadata"] = "unreadable: no bd 1.3.1 command exposes it"
	r.Uncovered["leases"] = "unreadable: no bd 1.3.1 command exposes it"

	if e.o.SkipProvenance {
		r.Uncovered["provenance_events"] = "unread: skipped"
	} else {
		prov := 0
		for _, b := range r.Beads {
			prov += b.Provenance
		}
		r.Uncovered["provenance_events"] = fmt.Sprint(prov)
	}

	if out, err := e.run(root, "events", "export"); err != nil {
		r.Uncovered["bd_events_journal"] = "unreadable: " + err.Error()
	} else {
		n := 0
		for _, ln := range bytes.Split(out, []byte("\n")) {
			if s := bytes.TrimSpace(ln); len(s) > 0 && s[0] == '{' {
				n++
			}
		}
		r.Uncovered["bd_events_journal"] = fmt.Sprint(n)
	}

	// Wisps and gates: anything `list` shows that `export` does not.
	out, err := e.run(root, "list", "--all", "--include-ephemeral", "--include-gates", "--json", "-n", "0")
	if err != nil {
		r.Uncovered["wisps"] = "unreadable: " + err.Error()
		return
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		r.Uncovered["wisps"] = "unreadable: " + err.Error()
		return
	}
	have := map[string]bool{}
	for _, b := range r.Beads {
		have[b.ID] = true
	}
	n := 0
	for _, x := range rows {
		if !have[x.ID] {
			n++
		}
	}
	r.Uncovered["wisps"] = fmt.Sprint(n)
}

// Nonzero lists uncovered tables that are unreadable, unread or hold rows.
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
