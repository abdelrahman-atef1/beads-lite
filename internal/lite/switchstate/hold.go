package switchstate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/steveyegge/beads/internal/lockfile"
	"github.com/steveyegge/beads/internal/storage/dbproxy/util"
)

// Freeze moves an announced switch to frozen. It refuses while a session is
// pending unless force is set. The caller must then start a lock holder
// (RunHolder) so upstream bd fails loudly on .beads/embeddeddolt/.lock.
func Freeze(beadsDir, by string, window time.Duration, force bool, now time.Time) (*State, error) {
	s, err := Read(beadsDir)
	if err != nil {
		return nil, err
	}
	if s == nil || s.State != StateAnnounced {
		return nil, errors.New("switchstate: freeze needs an announced switch")
	}
	if p := Pending(s, window, now); len(p) > 0 && !force {
		return nil, fmt.Errorf("switchstate: sessions still active and not acked: %v", p)
	}
	s.State, s.FreezeAt, s.By = StateFrozen, now.Format("15:04"), by
	return s, Write(beadsDir, s)
}

// RunHolder takes upstream's exclusive embedded lock, signals through the
// hold marker, and keeps the lock until the state is no longer "frozen" (flip
// or rollback removes it). It runs only during a freeze and polls one file;
// nothing runs the rest of the time.
func RunHolder(beadsDir string, poll time.Duration) error {
	dir := filepath.Join(beadsDir, "embeddeddolt")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("switchstate: %s not found; nothing to hold", dir)
	}
	lk, err := util.TryLock(filepath.Join(dir, ".lock"))
	if err != nil {
		if lockfile.IsLocked(err) {
			return fmt.Errorf("switchstate: another process holds %s/.lock (a bd command is running?)", dir)
		}
		return err
	}
	defer lk.Unlock()
	hold := HoldPath(beadsDir)
	if err := os.WriteFile(hold, []byte(fmt.Sprint(os.Getpid())), 0o600); err != nil {
		return err
	}
	defer os.Remove(hold)
	for {
		s, err := Read(beadsDir)
		if err == nil && (s == nil || s.State != StateFrozen) {
			return nil
		}
		time.Sleep(poll)
	}
}

// WaitHolding waits until the holder wrote its marker.
func WaitHolding(beadsDir string, timeout time.Duration) error {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if _, err := os.Stat(HoldPath(beadsDir)); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("switchstate: lock holder did not start in time")
}

// WaitReleased waits until the holder has exited (marker gone).
func WaitReleased(beadsDir string, timeout time.Duration) error {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if _, err := os.Stat(HoldPath(beadsDir)); errors.Is(err, os.ErrNotExist) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("switchstate: lock holder still running")
}
