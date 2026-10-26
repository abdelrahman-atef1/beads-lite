// Package switchstate carries the per-project switch notice and freeze
// (contracts/switch.md, research R8). The tracker is the only channel every
// session reaches, so the state lives in <beadsDir>/lite-switch.json and every
// fork command reads it at start (one stat when absent).
package switchstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Exit codes and states.
const (
	ExitFrozen     = 3
	StateAnnounced = "announced"
	StateFrozen    = "frozen"
	// DefaultWindow is how long a session may be silent before it no longer
	// blocks the freeze.
	DefaultWindow = 15 * time.Minute

	fileName = "lite-switch.json"
	holdName = "lite-switch.hold"
	timeFmt  = time.RFC3339
)

// Session is one active chat/agent in the project.
type Session struct {
	Who   string  `json:"who"`
	Seen  string  `json:"seen"`   // last activity (transcript mtime)
	AckAt *string `json:"ack_at"` // null until acknowledged
}

// State is the content of lite-switch.json.
type State struct {
	State       string    `json:"state"`
	Project     string    `json:"project"`
	AnnouncedAt string    `json:"announced_at"`
	FreezeAt    string    `json:"freeze_at"` // planned (announced) or actual (frozen), "HH:MM"
	Sessions    []Session `json:"sessions"`
	By          string    `json:"by"`
}

// FrozenError is returned for a refused write; the caller exits with ExitFrozen.
type FrozenError struct{ Project, Since, By string }

func (e *FrozenError) Error() string {
	return fmt.Sprintf("%s is frozen for the beads-lite switch (since %s, by %s); retry after it ends", e.Project, e.Since, e.By)
}

// Path returns the state file path.
func Path(beadsDir string) string { return filepath.Join(beadsDir, fileName) }

// HoldPath is the marker the lock holder writes once it owns the lock.
func HoldPath(beadsDir string) string { return filepath.Join(beadsDir, holdName) }

// Read returns the state, or (nil, nil) when no switch is in progress.
func Read(beadsDir string) (*State, error) {
	b, err := os.ReadFile(Path(beadsDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		// A damaged file must not silently allow writes during a freeze.
		return nil, fmt.Errorf("switchstate: %s unreadable: %w", Path(beadsDir), err)
	}
	return &s, nil
}

// Write stores the state atomically (temp file + rename).
func Write(beadsDir string, s *State) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path(beadsDir) + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path(beadsDir))
}

// Remove deletes the state file (flip, rollback). Missing is fine.
func Remove(beadsDir string) error {
	if err := os.Remove(Path(beadsDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Announce writes state "announced" with the sessions found active.
func Announce(beadsDir, project, freezeAt, by string, sessions []Session, now time.Time) (*State, error) {
	if project == "" || freezeAt == "" {
		return nil, errors.New("switchstate: project and freeze time are required")
	}
	cur, err := Read(beadsDir)
	if err != nil {
		return nil, err
	}
	if cur != nil && cur.State == StateFrozen {
		return nil, errors.New("switchstate: project is already frozen")
	}
	s := &State{State: StateAnnounced, Project: project, AnnouncedAt: now.Format(timeFmt),
		FreezeAt: freezeAt, Sessions: sessions, By: by}
	if s.Sessions == nil {
		s.Sessions = []Session{}
	}
	return s, Write(beadsDir, s)
}

// Ack records the actor's acknowledgement. An actor not on the list is added,
// so a session that started after the announcement can still stop the wait.
func Ack(beadsDir, actor string, now time.Time) error {
	s, err := Read(beadsDir)
	if err != nil {
		return err
	}
	if s == nil {
		return errors.New("switchstate: no switch announced")
	}
	t := now.Format(timeFmt)
	for i := range s.Sessions {
		if s.Sessions[i].Who == actor {
			s.Sessions[i].AckAt = &t
			return Write(beadsDir, s)
		}
	}
	s.Sessions = append(s.Sessions, Session{Who: actor, Seen: t, AckAt: &t})
	return Write(beadsDir, s)
}

// Pending lists sessions that still block the freeze: not acked and active
// within window. An unparseable "seen" counts as active (fail safe).
func Pending(s *State, window time.Duration, now time.Time) []string {
	var out []string
	for _, x := range s.Sessions {
		if x.AckAt != nil {
			continue
		}
		seen, err := time.Parse(timeFmt, x.Seen)
		if err == nil && now.Sub(seen) > window {
			continue
		}
		out = append(out, x.Who)
	}
	return out
}

// Check is called by every fork command after the project is resolved.
// announced: one stderr line, then proceeds. frozen: reads pass, writes get
// a *FrozenError. A missing file costs one failed open.
func Check(beadsDir string, write bool, stderr io.Writer) error {
	s, err := Read(beadsDir)
	if err != nil {
		if write {
			return err // cannot prove the project is not frozen
		}
		fmt.Fprintf(stderr, "beads-lite: warning: %v\n", err)
		return nil
	}
	if s == nil {
		return nil
	}
	switch s.State {
	case StateAnnounced:
		fmt.Fprintf(stderr, "beads-lite: %s switches at %s; finish, then run `bd lite-switch ack`\n", s.Project, s.FreezeAt)
	case StateFrozen:
		if write {
			return &FrozenError{Project: s.Project, Since: s.FreezeAt, By: s.By}
		}
	}
	return nil
}
