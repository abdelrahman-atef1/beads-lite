package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/beads/internal/lite/switchstate"
)

// company-88mo: calls handed to upstream bd (embedded-Dolt projects, the ones
// that are frozen during a switch) must obey the freeze like fork calls do.
func TestLiteFreezeGuard(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".beads")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	state := func(s string) {
		t.Helper()
		js := `{"state":"` + s + `","project":"p","freeze_at":"03:00","by":"sre"}`
		if err := os.WriteFile(switchstate.Path(dir), []byte(js), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var fe *switchstate.FrozenError
	state("frozen")
	for _, args := range [][]string{{"create", "x", "-p", "4"}, {"update", "p-1", "--claim"}, {"close", "p-1"}, {"comments", "add", "p-1", "hi"}, {"--json", "create", "x"}} {
		if err := liteFreezeGuard(dir, args); !errors.As(err, &fe) {
			t.Errorf("%v must be refused while frozen, got %v", args, err)
		}
	}
	for _, args := range [][]string{{"list", "--json"}, {"show", "p-1"}, {"ready"}, {"export"}, {"create", "x", "--dry-run"}} {
		if err := liteFreezeGuard(dir, args); err != nil {
			t.Errorf("%v must be allowed while frozen: %v", args, err)
		}
	}
	state("announced")
	if err := liteFreezeGuard(dir, []string{"create", "x"}); err != nil {
		t.Errorf("announced must not refuse: %v", err)
	}
	os.Remove(switchstate.Path(dir))
	if err := liteFreezeGuard(dir, []string{"create", "x"}); err != nil {
		t.Errorf("no state must allow: %v", err)
	}
}
