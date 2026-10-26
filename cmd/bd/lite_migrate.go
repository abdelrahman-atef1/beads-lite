package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/migration/sqlitelite"
)

// bd migrate-to-sqlite (W8, T020): read a frozen Dolt-backed project through
// upstream bd 1.3.1, write a NEW .beads/beads.db, verify it, and only then
// move it into place. It never writes embeddeddolt/ and never overwrites an
// existing beads.db. Also reachable as `bd lite-switch migrate`.

func init() {
	liteMigrate = runLiteMigrate

	var bdBin, report, accept, workDir string
	var workers int
	var skipShow bool
	cmd := &cobra.Command{
		Use:         "migrate-to-sqlite",
		Short:       "Copy a (frozen) Dolt store into a new .beads/beads.db and verify it",
		Annotations: map[string]string{skipStoreAnnotation: "1"},
		Run: func(cmd *cobra.Command, _ []string) {
			d, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			rep, err := liteMigrateRun(d, liteMigrateFlags{bd: bdBin, accept: splitList(accept), work: workDir, workers: workers, skipShow: skipShow})
			saveMigrateReport(rep, report)
			if rep != nil {
				fmt.Println(rep.Text())
			}
			if err != nil {
				switchFail(err)
			}
		},
	}
	cmd.Flags().StringVar(&bdBin, "bd", "", "upstream bd 1.3.1 executable (default: BD_LITE_UPSTREAM, then bd on PATH)")
	cmd.Flags().StringVar(&report, "report", "", "write the JSON report to this file")
	cmd.Flags().StringVar(&accept, "accept-uncovered", "", "comma list of tables the extractor cannot read that you accept as empty (recorded in the report)")
	cmd.Flags().StringVar(&workDir, "work-dir", "", "folder for the extractor's copies (default: temp)")
	cmd.Flags().IntVar(&workers, "workers", 4, "parallel copies")
	cmd.Flags().BoolVar(&skipShow, "skip-show-diff", false, "development only: skip the show --json comparison")
	rootCmd.AddCommand(cmd)
}

type liteMigrateFlags struct {
	bd, work string
	accept   []string
	workers  int
	skipShow bool
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func saveMigrateReport(rep *sqlitelite.MigrateReport, path string) {
	if rep == nil || path == "" {
		return
	}
	if b, err := json.MarshalIndent(rep, "", "  "); err == nil {
		_ = os.WriteFile(path, b, 0o644)
	}
}

// runLiteMigrate is the switchstate hook: default options, report as text.
func runLiteMigrate(beadsDir string) (string, error) {
	rep, err := liteMigrateRun(beadsDir, liteMigrateFlags{workers: 4})
	text := ""
	if rep != nil {
		text = rep.Text()
		saveMigrateReport(rep, filepath.Join(beadsDir, "lite-migrate-report.json"))
	}
	return text, err
}

func liteMigrateRun(beadsDir string, f liteMigrateFlags) (*sqlitelite.MigrateReport, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	up, err := resolveUpstreamBd(f.bd, self)
	if err != nil {
		return nil, err
	}
	return sqlitelite.Migrate(context.Background(), sqlitelite.MigrateOptions{
		Project: filepath.Dir(beadsDir), BdBin: up, ForkBin: self, WorkDir: f.work,
		Workers: f.workers, Accept: f.accept, SkipShow: f.skipShow,
	})
}

// resolveUpstreamBd finds upstream bd 1.3.1 and refuses to run the fork as its own source.
func resolveUpstreamBd(flag, self string) (string, error) {
	p := flag
	if p == "" {
		p = os.Getenv("BD_LITE_UPSTREAM")
	}
	if p == "" {
		found, err := exec.LookPath("bd")
		if err != nil {
			return "", errors.New("upstream bd 1.3.1 not found: pass --bd or set BD_LITE_UPSTREAM")
		}
		p = found
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if sameFile(abs, self) {
		return "", errors.New("the upstream bd path points at this binary; pass --bd or set BD_LITE_UPSTREAM to bd 1.3.1")
	}
	return abs, nil
}

func sameFile(a, b string) bool {
	sa, e1 := os.Stat(a)
	sb, e2 := os.Stat(b)
	return e1 == nil && e2 == nil && os.SameFile(sa, sb)
}

var _ = beads.FindBeadsDir
