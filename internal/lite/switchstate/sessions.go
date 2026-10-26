package switchstate

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ListSessions returns the Claude Code project folders with a transcript
// changed in the last `since` under claudeProjects (~/.claude/projects).
// Claude names a folder after the working path with separators and ':' turned
// into '-', so a folder matches when its name starts with the mangled path of
// any of dirs (project root or a worktree). Read-only.
func ListSessions(claudeProjects string, dirs []string, since time.Duration, now time.Time) []Session {
	out := []Session{}
	ents, err := os.ReadDir(claudeProjects)
	if err != nil {
		return out
	}
	var prefixes []string
	for _, d := range dirs {
		prefixes = append(prefixes, strings.ToLower(Mangle(d)))
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		match := false
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				match = true
			}
		}
		if !match {
			continue
		}
		var latest time.Time
		files, _ := filepath.Glob(filepath.Join(claudeProjects, e.Name(), "*.jsonl"))
		for _, f := range files {
			fi, err := os.Stat(f)
			if err != nil || now.Sub(fi.ModTime()) > since {
				continue
			}
			if fi.ModTime().After(latest) {
				latest = fi.ModTime()
			}
		}
		if !latest.IsZero() {
			out = append(out, Session{Who: e.Name(), Seen: latest.Format(timeFmt)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Who < out[j].Who })
	return out
}

// Mangle turns a path into Claude Code's project folder name.
func Mangle(p string) string {
	return strings.NewReplacer("\\", "-", "/", "-", ":", "-", " ", "-", "_", "-", ".", "-").Replace(p)
}
