package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/steveyegge/beads/internal/beads"
	"github.com/steveyegge/beads/internal/configfile"
	"github.com/steveyegge/beads/internal/lite/switchstate"
)

// liteMigrate is set by the W8 migration tool (T020). Until then the migrate
// step reports that it is not available instead of pretending.
var liteMigrate func(beadsDir string) (report string, err error)

// bd lite-switch: per-project notice, freeze, migrate, flip, rollback
// (specs/001-fast-compact-tracker/contracts/switch.md). Run by SRE.
var liteSwitchCmd = &cobra.Command{
	Use:         "lite-switch",
	Short:       "Switch a project to beads-lite: announce, ack, freeze, migrate, flip, rollback",
	Annotations: map[string]string{skipStoreAnnotation: "1"},
}

func switchDir() (string, error) {
	d := beads.FindBeadsDir()
	if d == "" {
		return "", errors.New("no .beads directory found from here")
	}
	return d, nil
}

func switchActor(flag string) string {
	if flag != "" {
		return flag
	}
	for _, k := range []string{"BD_ACTOR", "USERNAME", "USER"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return "unknown"
}

func switchFail(err error) {
	fmt.Fprintln(os.Stderr, "Error:", err)
	os.Exit(1)
}

func init() {
	var at, project, by, claudeDir string
	var dirs []string
	var window time.Duration
	var force bool

	announce := &cobra.Command{
		Use:   "announce --at HH:MM --project NAME",
		Short: "Write the notice; list sessions active in the last 2 h",
		Run: func(cmd *cobra.Command, _ []string) {
			bd, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			if claudeDir == "" {
				home, _ := os.UserHomeDir()
				claudeDir = filepath.Join(home, ".claude", "projects")
			}
			if len(dirs) == 0 {
				dirs = []string{filepath.Dir(bd)}
			}
			ss := switchstate.ListSessions(claudeDir, dirs, 2*time.Hour, time.Now())
			s, err := switchstate.Announce(bd, project, at, switchActor(by), ss, time.Now())
			if err != nil {
				switchFail(err)
			}
			fmt.Printf("announced %s for %s; %d active session(s):\n", s.Project, s.FreezeAt, len(s.Sessions))
			for _, x := range s.Sessions {
				fmt.Printf("  %s (last seen %s)\n", x.Who, x.Seen)
			}
		},
	}
	announce.Flags().StringVar(&at, "at", "", "planned freeze time, HH:MM")
	announce.Flags().StringVar(&project, "project", "", "project name shown in the notice")
	announce.Flags().StringVar(&by, "by", "", "who runs the switch (default $BD_ACTOR)")
	announce.Flags().StringVar(&claudeDir, "claude-projects", "", "Claude Code transcripts folder (default ~/.claude/projects)")
	announce.Flags().StringSliceVar(&dirs, "dir", nil, "project folder or worktree to scan (repeatable; default: the project folder)")

	ack := &cobra.Command{
		Use:   "ack",
		Short: "Tell the switch tool this session has finished its work",
		Run: func(cmd *cobra.Command, _ []string) {
			bd, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			a := switchActor(by)
			if err := switchstate.Ack(bd, a, time.Now()); err != nil {
				switchFail(err)
			}
			fmt.Println("acknowledged:", a)
		},
	}
	ack.Flags().StringVar(&by, "as", "", "session name to record (default $BD_ACTOR); use the name listed by announce")

	status := &cobra.Command{
		Use:   "status",
		Short: "Show the switch state and who still blocks the freeze",
		Run: func(cmd *cobra.Command, _ []string) {
			bd, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			s, err := switchstate.Read(bd)
			if err != nil {
				switchFail(err)
			}
			if s == nil {
				fmt.Println("no switch in progress")
				return
			}
			fmt.Printf("%s: %s at %s by %s\n", s.Project, s.State, s.FreezeAt, s.By)
			for _, x := range s.Sessions {
				ackd := "waiting"
				if x.AckAt != nil {
					ackd = "acked " + *x.AckAt
				}
				fmt.Printf("  %s: %s\n", x.Who, ackd)
			}
			if p := switchstate.Pending(s, switchstate.DefaultWindow, time.Now()); len(p) > 0 && s.State == switchstate.StateAnnounced {
				fmt.Println("blocking the freeze:", p)
			}
		},
	}

	freeze := &cobra.Command{
		Use:   "freeze",
		Short: "Refuse fork writes and hold upstream's embedded lock",
		Run: func(cmd *cobra.Command, _ []string) {
			bd, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			if _, err := switchstate.Freeze(bd, switchActor(by), window, force, time.Now()); err != nil {
				switchFail(err)
			}
			// The lock must outlive this command: a short-lived hidden child holds
			// it until flip or rollback removes the state file.
			exe, err := os.Executable()
			if err == nil {
				c := exec.Command(exe, "lite-switch", "hold")
				c.Dir = filepath.Dir(bd)
				detach(c)
				err = c.Start()
			}
			if err == nil {
				err = switchstate.WaitHolding(bd, 15*time.Second)
			}
			if err != nil {
				_ = switchstate.Remove(bd) // never leave a freeze with no lock behind it
				switchFail(fmt.Errorf("freeze undone: %w", err))
			}
			fmt.Println("frozen; upstream lock held")
		},
	}
	freeze.Flags().StringVar(&by, "by", "", "who runs the switch (default $BD_ACTOR)")
	freeze.Flags().DurationVar(&window, "window", switchstate.DefaultWindow, "a session silent this long no longer blocks the freeze")
	freeze.Flags().BoolVar(&force, "force", false, "freeze although sessions have not acked (owner decision)")

	hold := &cobra.Command{
		Use:    "hold",
		Short:  "Internal: hold upstream's lock while frozen",
		Hidden: true,
		Run: func(cmd *cobra.Command, _ []string) {
			bd, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			if err := switchstate.RunHolder(bd, 2*time.Second); err != nil {
				switchFail(err)
			}
		},
	}

	migrate := &cobra.Command{
		Use:   "migrate",
		Short: "Copy the frozen Dolt store into a new beads.db and verify it",
		Run: func(cmd *cobra.Command, _ []string) {
			bd, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			s, err := switchstate.Read(bd)
			if err != nil {
				switchFail(err)
			}
			if s == nil || s.State != switchstate.StateFrozen {
				switchFail(errors.New("migrate runs only while frozen (nothing is copied before freeze)"))
			}
			if liteMigrate == nil {
				switchFail(errors.New("migrate tool not built into this binary yet (task T020)"))
			}
			report, err := liteMigrate(bd)
			if report != "" {
				fmt.Println(report)
			}
			if err != nil {
				switchFail(err)
			}
		},
	}

	flip := &cobra.Command{
		Use:   "flip",
		Short: "Make sqlite the backend, end the freeze, release the lock",
		Run: func(cmd *cobra.Command, _ []string) {
			bd, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			if err := switchFlip(bd); err != nil {
				switchFail(err)
			}
			fmt.Println("flipped to sqlite; restart the board, Telegram bridge and other long-running callers")
		},
	}

	rollback := &cobra.Command{
		Use:   "rollback",
		Short: "Back to dolt: archive the new beads.db, end the freeze",
		Run: func(cmd *cobra.Command, _ []string) {
			bd, err := switchDir()
			if err != nil {
				switchFail(err)
			}
			if err := switchRollback(bd, time.Now()); err != nil {
				switchFail(err)
			}
			fmt.Println("rolled back to dolt; the project is as before the freeze")
		},
	}

	liteSwitchCmd.AddCommand(announce, ack, status, freeze, hold, migrate, flip, rollback)
	rootCmd.AddCommand(liteSwitchCmd)
}

// switchFlip sets backend sqlite, then removes the state (ending the freeze)
// and waits for the lock holder to exit. embeddeddolt/ is never touched.
func switchFlip(beadsDir string) error {
	s, err := switchstate.Read(beadsDir)
	if err != nil {
		return err
	}
	if s == nil || s.State != switchstate.StateFrozen {
		return errors.New("flip needs a frozen project")
	}
	if _, err := os.Stat(filepath.Join(beadsDir, "beads.db")); err != nil {
		return errors.New("flip needs the migrated beads.db; run migrate first")
	}
	return setBackendAndEnd(beadsDir, "sqlite")
}

// switchRollback sets backend dolt, archives any new beads.db (moved, never
// deleted), and ends the freeze.
func switchRollback(beadsDir string, now time.Time) error {
	if db := filepath.Join(beadsDir, "beads.db"); fileExists(beadsDir, "beads.db") {
		dst := filepath.Join(beadsDir, "lite-archive")
		if err := os.MkdirAll(dst, 0o700); err != nil {
			return err
		}
		if err := os.Rename(db, filepath.Join(dst, "beads.db."+now.Format("20060102-150405"))); err != nil {
			return err
		}
	}
	return setBackendAndEnd(beadsDir, "dolt")
}

func setBackendAndEnd(beadsDir, backend string) error {
	cfg, err := configfile.Load(beadsDir)
	if err != nil {
		return err
	}
	if cfg == nil {
		return errors.New("metadata.json not found")
	}
	cfg.Backend = backend
	if err := cfg.Save(beadsDir); err != nil {
		return err
	}
	if err := switchstate.Remove(beadsDir); err != nil {
		return err
	}
	return switchstate.WaitReleased(beadsDir, 15*time.Second)
}
