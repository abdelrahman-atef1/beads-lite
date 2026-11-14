package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// beads-lite-1bo5.12: an explicit --db must be an existing beads store.
func TestValidateExplicitDBFlag(t *testing.T) {
	root := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.db", append([]byte("SQLite format 3\x00"), make([]byte, 64)...))
	text := write("notes.txt", []byte("hello world, not a db at all"))
	empty := write("empty.db", nil)
	storeDir := filepath.Join(root, "proj", ".beads")
	write("proj/.beads/metadata.json", []byte("{}"))
	plainDir := filepath.Join(root, "plain")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missingAbs := filepath.Join(root, "typo", "x.db")

	ok := []string{good, storeDir}
	for _, p := range ok {
		if err := validateExplicitDBFlag(p); err != nil {
			t.Errorf("%q should be accepted: %v", p, err)
		}
	}
	bad := map[string]string{
		missingAbs: "database not found",
		text:       "not a beads database",
		empty:      "not a beads database",
		plainDir:   "not a beads database",
	}
	for p, want := range bad {
		err := validateExplicitDBFlag(p)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: want error containing %q, got %v", p, want, err)
		}
	}

	// Relative paths: existing store accepted once made absolute, missing rejected.
	t.Chdir(root)
	if err := validateExplicitDBFlag(absoluteExistingDBFlag("good.db")); err != nil {
		t.Errorf("relative existing: %v", err)
	}
	for _, rel := range []string{"nope.db", filepath.Join("typo", "x.db")} {
		err := validateExplicitDBFlag(absoluteExistingDBFlag(rel))
		if err == nil || !strings.Contains(err.Error(), "database not found") {
			t.Errorf("relative missing %q: got %v", rel, err)
		}
	}
}

func TestValidateEnvDB(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.db")
	if err := os.WriteFile(good, append([]byte("SQLite format 3\x00"), make([]byte, 32)...), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"BEADS_DB", "BD_DB"} {
		t.Setenv("BEADS_DB", "")
		t.Setenv("BD_DB", "")
		if err := validateEnvDB(); err != nil {
			t.Errorf("unset must pass: %v", err)
		}
		t.Setenv(key, good)
		if err := validateEnvDB(); err != nil {
			t.Errorf("%s good: %v", key, err)
		}
		t.Setenv(key, filepath.Join(root, "typo", "x.db"))
		if err := validateEnvDB(); err == nil || !strings.Contains(err.Error(), "database not found") {
			t.Errorf("%s missing: %v", key, err)
		}
		txt := filepath.Join(root, "n.txt")
		_ = os.WriteFile(txt, []byte("not a db at all, just text"), 0o600)
		t.Setenv(key, txt)
		if err := validateEnvDB(); err == nil {
			t.Errorf("%s non-store must fail", key)
		}
	}
}
