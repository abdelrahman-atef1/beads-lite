package switchstate

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/lockfile"
	"github.com/steveyegge/beads/internal/storage/dbproxy/util"
)

var t0 = time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)

func newDir(t *testing.T) string {
	t.Helper()
	d := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(filepath.Join(d, "embeddeddolt"), 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}

func sess(who string, ago time.Duration) Session {
	return Session{Who: who, Seen: t0.Add(-ago).Format(timeFmt)}
}

func TestCheckNoFileIsSilent(t *testing.T) {
	var b bytes.Buffer
	if err := Check(newDir(t), true, &b); err != nil || b.Len() != 0 {
		t.Fatalf("err=%v out=%q", err, b.String())
	}
}

func TestAnnouncedPrintsNoticeAndAllowsWrites(t *testing.T) {
	d := newDir(t)
	if _, err := Announce(d, "rayeh", "14:30", "sre", nil, t0); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := Check(d, true, &b); err != nil {
		t.Fatal(err)
	}
	want := "beads-lite: rayeh switches at 14:30; finish, then run `bd lite-switch ack`\n"
	if b.String() != want {
		t.Fatalf("got %q", b.String())
	}
}

func TestFrozenRefusesWritesAllowsReads(t *testing.T) {
	d := newDir(t)
	Announce(d, "rayeh", "14:30", "sre", nil, t0)
	if _, err := Freeze(d, "sre", DefaultWindow, false, t0); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	err := Check(d, true, &b)
	var fe *FrozenError
	if !errors.As(err, &fe) {
		t.Fatalf("want FrozenError, got %v", err)
	}
	if err.Error() != "rayeh is frozen for the beads-lite switch (since 10:00, by sre); retry after it ends" {
		t.Fatal(err.Error())
	}
	if err := Check(d, false, &b); err != nil {
		t.Fatalf("read refused: %v", err)
	}
}

func TestCorruptStateRefusesWrites(t *testing.T) {
	d := newDir(t)
	os.WriteFile(Path(d), []byte("{nope"), 0o600)
	var b bytes.Buffer
	if Check(d, true, &b) == nil {
		t.Fatal("write allowed with unreadable state")
	}
	if Check(d, false, &b) != nil || !strings.Contains(b.String(), "warning") {
		t.Fatal("read should pass with a warning")
	}
}

func TestFreezeWaitsForSessions(t *testing.T) {
	d := newDir(t)
	Announce(d, "rayeh", "14:30", "sre", []Session{sess("a", time.Minute), sess("b", 2*time.Hour)}, t0)
	if _, err := Freeze(d, "sre", DefaultWindow, false, t0); err == nil || !strings.Contains(err.Error(), "[a]") {
		t.Fatalf("a is active and unacked, want refusal naming only a, got %v", err)
	}
	if err := Ack(d, "a", t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Freeze(d, "sre", DefaultWindow, false, t0); err != nil {
		t.Fatalf("after ack (b idle): %v", err)
	}
}

func TestFreezeForceAndNeedsAnnounce(t *testing.T) {
	d := newDir(t)
	if _, err := Freeze(d, "sre", DefaultWindow, true, t0); err == nil {
		t.Fatal("freeze without announce must fail")
	}
	Announce(d, "p", "14:30", "sre", []Session{sess("a", time.Minute)}, t0)
	if _, err := Freeze(d, "sre", DefaultWindow, true, t0); err != nil {
		t.Fatal(err)
	}
	if _, err := Announce(d, "p", "15:00", "sre", nil, t0); err == nil {
		t.Fatal("announce over a freeze must fail")
	}
}

func TestAckAddsLateSession(t *testing.T) {
	d := newDir(t)
	Announce(d, "p", "14:30", "sre", nil, t0)
	if err := Ack(d, "late", t0); err != nil {
		t.Fatal(err)
	}
	s, _ := Read(d)
	if len(s.Sessions) != 1 || s.Sessions[0].AckAt == nil {
		t.Fatalf("%+v", s.Sessions)
	}
	if Ack(newDir(t), "x", t0) == nil {
		t.Fatal("ack without announce must fail")
	}
}

func TestHolderHoldsUpstreamLockUntilStateRemoved(t *testing.T) {
	d := newDir(t)
	Announce(d, "p", "14:30", "sre", nil, t0)
	if _, err := Freeze(d, "sre", DefaultWindow, false, t0); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- RunHolder(d, 20*time.Millisecond) }()
	if err := WaitHolding(d, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	// What upstream bd does at open: must now fail loudly.
	_, err := util.TryLock(filepath.Join(d, "embeddeddolt", ".lock"))
	if err == nil || !lockfile.IsLocked(err) {
		t.Fatalf("upstream lock should be held, got %v", err)
	}
	Remove(d) // flip or rollback
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("holder did not release")
	}
	if err := WaitReleased(d, 5*time.Second); err != nil {
		t.Fatal(err)
	}
	lk, err := util.TryLock(filepath.Join(d, "embeddeddolt", ".lock"))
	if err != nil {
		t.Fatalf("lock should be free after release: %v", err)
	}
	lk.Unlock()
}

func TestHolderNeverCreatesStore(t *testing.T) {
	d := filepath.Join(t.TempDir(), ".beads")
	os.MkdirAll(d, 0o700)
	if RunHolder(d, time.Millisecond) == nil {
		t.Fatal("must refuse when embeddeddolt/ is absent")
	}
	if _, err := os.Stat(filepath.Join(d, "embeddeddolt")); err == nil {
		t.Fatal("created embeddeddolt/")
	}
}

func TestListSessions(t *testing.T) {
	cp := t.TempDir()
	mk := func(name string, age time.Duration) {
		dir := filepath.Join(cp, name)
		os.MkdirAll(dir, 0o700)
		f := filepath.Join(dir, "s.jsonl")
		os.WriteFile(f, []byte("{}"), 0o600)
		os.Chtimes(f, t0.Add(-age), t0.Add(-age))
	}
	mk("E--Projects-rayeh", time.Hour)
	mk("E--Projects-rayeh-wt-1", 30*time.Minute)
	mk("E--Projects-rayeh-old", 5*time.Hour)
	mk("E--Projects-fatura", time.Minute)
	got := ListSessions(cp, []string{`E:\Projects\rayeh`}, 2*time.Hour, t0)
	if len(got) != 2 || got[0].Who != "E--Projects-rayeh" || got[1].Who != "E--Projects-rayeh-wt-1" {
		t.Fatalf("%+v", got)
	}
}
