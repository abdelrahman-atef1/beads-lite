package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

func TestMovedBeadsCheck_FlagValuesAndPrefixless(t *testing.T) {
	p := writeManifest(t)
	for _, a := range []string{"blocks:company-5c7.282", "x,company-5c7.282", "5c7.282"} {
		if got := movedBeadsCheck(p, []string{a}); !strings.Contains(got, "filmmaker-5c7.282") {
			t.Fatalf("%q not caught: %q", a, got)
		}
	}
	if got := movedBeadsCheck(p, []string{"blocks:company-other", "5c7"}); got != "" {
		t.Fatalf("false positive: %q", got)
	}
}

func TestMovedBeadsCheck_FreeTextNotBlocked(t *testing.T) {
	p := writeManifest(t)
	for _, a := range []string{
		"company-5c7.282: see new", "see company-5c7.282 for details", "id: company-5c7.282",
		"moved 5c7.282, done", "line1\ncompany-5c7.282", "blocks:company-other and company-5c7.282",
	} {
		if got := movedBeadsCheck(p, []string{"comments", "add", "company-other", a}); got != "" {
			t.Fatalf("free text %q blocked: %q", a, got)
		}
	}
	// id slots still block, wherever they sit
	if got := movedBeadsCheck(p, []string{"comments", "add", "company-5c7.282", "text"}); got == "" {
		t.Fatal("positional id not blocked")
	}
}

func TestEnforceFlags_TextFlagsIgnoredIdFlagsChecked(t *testing.T) {
	p := writeManifest(t)
	mk := func(name, val string) *cobra.Command {
		c := &cobra.Command{Use: "x"}
		c.Flags().String(name, "", "")
		_ = c.Flags().Set(name, val)
		return c
	}
	collect := func(c *cobra.Command) string {
		var toks []string
		c.Flags().Visit(func(f *pflag.Flag) {
			if !freeTextFlags[f.Name] {
				toks = append(toks, f.Value.String())
			}
		})
		return movedBeadsCheck(p, toks)
	}
	for _, n := range []string{"notes", "design", "description", "acceptance", "title", "append-notes", "message", "reason"} {
		if got := collect(mk(n, "company-5c7.282")); got != "" {
			t.Fatalf("--%s blocked: %q", n, got)
		}
	}
	for _, n := range []string{"parent", "deps", "of", "with", "id"} {
		if got := collect(mk(n, "company-5c7.282")); got == "" {
			t.Fatalf("--%s not blocked", n)
		}
	}
}
