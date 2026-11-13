package main

import (
	"os"
	"path/filepath"
	"testing"
)

// beads-lite-1bo5.11: a relative --db must become absolute against the cwd, so
// later steps cannot fall back to the nearest .beads of that cwd.
func TestAbsoluteExistingDBFlag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "copy.db"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	want, _ := filepath.Abs("copy.db")
	if got := absoluteExistingDBFlag("copy.db"); got != want {
		t.Errorf("relative existing: got %q want %q", got, want)
	}
	for _, in := range []string{"", "mydb", want} {
		if got := absoluteExistingDBFlag(in); got != in {
			t.Errorf("%q must be unchanged, got %q", in, got)
		}
	}
}
