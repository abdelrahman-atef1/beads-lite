package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "moved.tsv")
	body := "# old\tnew\trepo\ncompany-5c7.282\tfilmmaker-5c7.282\tfilmmaker\nbroken line\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMovedBeadsCheck_HitMissAndNoFile(t *testing.T) {
	p := writeManifest(t)
	msg := movedBeadsCheck(p, []string{"add", "COMPANY-5c7.282", "text"})
	if !strings.Contains(msg, "moved to filmmaker-5c7.282 in filmmaker") {
		t.Fatalf("want moved message, got %q", msg)
	}
	if got := movedBeadsCheck(p, []string{"company-other"}); got != "" {
		t.Fatalf("unmoved id flagged: %q", got)
	}
	if got := movedBeadsCheck(filepath.Join(t.TempDir(), "none.tsv"), []string{"company-5c7.282"}); got != "" {
		t.Fatalf("missing manifest must be a no-op, got %q", got)
	}
}
