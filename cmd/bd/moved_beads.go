package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// movedBeadsCheck returns an error message for the first arg that is a moved
// id, or "". Cheap pre-filter: only args shaped like an id are looked up.
func movedBeadsCheck(path string, args []string) string {
	var m map[string]movedBead
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" || strings.ContainsAny(a, " \t\n") || !strings.Contains(a, "-") {
			continue
		}
		if m == nil {
			m = loadMovedBeads(path)
			if len(m) == 0 {
				return ""
			}
		}
		if mv, ok := m[strings.ToLower(a)]; ok {
			return fmt.Sprintf("Issue %s moved to %s in %s (run bd there; nothing was changed)", a, mv.NewID, mv.Repo)
		}
	}
	return ""
}

// idCommands are the commands whose positional args are bead ids (or, for
// "comments", "label", "dep": a sub-verb followed by ids).
var idCommands = map[string]bool{
	"show": true, "update": true, "close": true, "reopen": true, "comments": true,
	"comment": true, "label": true, "dep": true, "delete": true, "defer": true,
	"undefer": true, "edit": true, "children": true, "history": true,
	"claim": true, "assign": true, "priority": true, "status": true, "tag": true,
	"rename": true, "move": true, "refs": true, "set-state": true,
}

func enforceNotMoved(cmdName string, args []string, idFlag []string) {
	if !idCommands[cmdName] {
		return
	}
	if msg := movedBeadsCheck(movedBeadsPath(), append(append([]string{}, args...), idFlag...)); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(1)
	}
}
