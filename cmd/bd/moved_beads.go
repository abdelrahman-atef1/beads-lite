package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Moved beads (company-8ce8.14.4). When a project's beads are split out of
// ~/company into their own repo, the old ids are listed in a manifest:
//
//	<old id> TAB <new id> TAB <repo>      ('#' starts a comment line)
//
// A bd command that names a moved id fails loudly, before any store is
// opened, so a write can never land under the old id.
// Missing manifest = feature off, zero cost beyond one failed open.

type movedBead struct{ NewID, Repo string }

func movedBeadsPath() string {
	if p := os.Getenv("BD_MOVED_BEADS_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "company", "knowledge", "moved-beads.tsv")
}

// loadMovedBeads reads the manifest. Any problem (no file, bad line) yields
// what could be read; it never errors, so bd keeps working without it.
func loadMovedBeads(path string) map[string]movedBead {
	out := map[string]movedBead{}
	if path == "" {
		return out
	}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) < 3 || strings.TrimSpace(cols[0]) == "" {
			continue
		}
		out[strings.ToLower(strings.TrimSpace(cols[0]))] = movedBead{
			NewID: strings.TrimSpace(cols[1]), Repo: strings.TrimSpace(cols[2]),
		}
	}
	return out
}

// movedBeadsCheck returns an error message for the first token that is a
// moved id, or "". Tokens may be "blocks:<id>" or "a,b" (flag values), and a
// prefix-less id ("j1fq.12.60.7") matches its "<prefix>-j1fq.12.60.7" row.
func movedBeadsCheck(path string, args []string) string {
	var m map[string]movedBead
	var short map[string]string
	for _, a := range args {
		// Free text (a comment, title, note) has whitespace; an id slot never does.
		if strings.ContainsAny(strings.TrimSpace(a), " \t\r\n") {
			continue
		}
		for _, tok := range strings.FieldsFunc(a, func(r rune) bool { return r == ',' || r == ':' }) {
			tok = strings.TrimSpace(tok)
			if tok == "" || strings.ContainsAny(tok, " \t\n") {
				continue
			}
			if m == nil {
				m = loadMovedBeads(path)
				if len(m) == 0 {
					return ""
				}
				short = map[string]string{}
				for k := range m {
					if i := strings.Index(k, "-"); i >= 0 {
						short[k[i+1:]] = k
					}
				}
			}
			key := strings.ToLower(tok)
			if _, ok := m[key]; !ok {
				if full, ok2 := short[key]; ok2 {
					key = full
				} else {
					continue
				}
			}
			mv := m[key]
			return fmt.Sprintf("Issue %s moved to %s in %s (run bd there; nothing was changed)", tok, mv.NewID, mv.Repo)
		}
	}
	return ""
}

// freeTextFlags hold prose, never ids: a moved id inside them is just a mention.
var freeTextFlags = map[string]bool{
	"title": true, "description": true, "notes": true, "append-notes": true,
	"design": true, "acceptance": true, "message": true, "reason": true,
	"comment": true, "text": true, "body": true, "context": true, "spec": true,
	"close-reason": true, "resolution": true, "label": true, "labels": true,
	"external-ref": true, "assignee": true, "owner": true, "metadata": true,
}

// enforceNotMoved checks every positional arg and every flag value the user
// set (--parent, --deps, --of, --with, --id, ...), for every command: one
// central place, so no write path can land on a moved id.
func enforceNotMoved(cmd *cobra.Command, args []string) {
	toks := append([]string{}, args...)
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if freeTextFlags[f.Name] {
			return
		}
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			toks = append(toks, sv.GetSlice()...)
		} else if f.Value.Type() == "string" {
			toks = append(toks, f.Value.String())
		}
	})
	if msg := movedBeadsCheck(movedBeadsPath(), toks); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(1)
	}
}
