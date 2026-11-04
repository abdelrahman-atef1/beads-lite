package compact

import (
	"fmt"
	"strings"
	"time"

	"github.com/steveyegge/beads/internal/foldtext"
	"github.com/steveyegge/beads/internal/types"
)

// Query selects and shapes the comments `bd comments` prints when any query
// flag is set (beads-lite-1bo5.6.46.5). Comment numbers (#n, oldest = 1) are
// those of the whole list, whatever filter is on.
type Query struct {
	Titles   bool      // --titles: one line per comment
	Last     int       // --last N: N newest, folded (0 = off)
	N        int       // --n K: comment #K, exact text (0 = off)
	Sections []string  // --section: print only the matching '## ' sections
	Since    time.Time // --since: comments created at or after (zero = off)
	Author   string    // --author: exact name, case-insensitive ("" = off)
}

// Active reports whether any query flag is set.
func (q Query) Active() bool {
	return q.Titles || q.Last > 0 || q.N > 0 || len(q.Sections) > 0 || !q.Since.IsZero() || q.Author != ""
}

// ParseSince accepts YYYY-MM-DD, or a age such as 12h, 3d, 2w ("ago" from now).
func ParseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	if len(s) > 1 {
		var n int
		if _, err := fmt.Sscanf(s[:len(s)-1], "%d", &n); err == nil && n >= 0 {
			switch s[len(s)-1] {
			case 'h':
				return now.Add(-time.Duration(n) * time.Hour), nil
			case 'd':
				return now.AddDate(0, 0, -n), nil
			case 'w':
				return now.AddDate(0, 0, -7*n), nil
			}
		}
	}
	return time.Time{}, fmt.Errorf("--since %q: use YYYY-MM-DD or an age like 12h, 3d, 2w", s)
}

// Comments renders the comment list for id under q. Order of work: filter by
// author and date, then --n, else --titles / --last / --section.
func Comments(id string, cmts []*types.Comment, q Query) (string, error) {
	type item struct {
		idx int
		c   *types.Comment
	}
	var all, sel []item
	for i, c := range cmts {
		it := item{i + 1, c}
		all = append(all, it)
		if q.Author != "" && !strings.EqualFold(c.Author, q.Author) {
			continue
		}
		if !q.Since.IsZero() && c.CreatedAt.Before(q.Since) {
			continue
		}
		sel = append(sel, it)
	}
	if q.N > 0 {
		if q.N > len(all) {
			return "", fmt.Errorf("%s has %d comments, no #%d", id, len(all), q.N)
		}
		c := all[q.N-1].c
		return fmt.Sprintf("#%d %s %s:\n%s\n", q.N, day(c.CreatedAt), c.Author, c.Text), nil
	}
	if len(sel) == 0 {
		return fmt.Sprintf("No comments on %s match\n", id), nil
	}
	hidden := 0
	if q.Last > 0 && len(sel) > q.Last {
		hidden = len(sel) - q.Last
		sel = sel[hidden:]
	}
	var b strings.Builder
	for _, it := range sel {
		c := it.c
		switch {
		case len(q.Sections) > 0:
			d := foldtext.Parse(strings.TrimSpace(c.Text))
			var out []string
			for _, s := range d.Sections {
				if s.Match(q.Sections) {
					out = append(out, "## "+s.Name, s.Body)
				}
			}
			if len(out) == 0 {
				continue
			}
			fmt.Fprintf(&b, "- #%d %s %s:\n%s\n", it.idx, day(c.CreatedAt), c.Author, indent(strings.Join(out, "\n")))
		case q.Titles:
			fmt.Fprintf(&b, "#%d %s %s: %s\n", it.idx, day(c.CreatedAt), c.Author, Title(c.Text))
		default:
			fmt.Fprintf(&b, "- #%d %s %s:\n%s\n", it.idx, day(c.CreatedAt), c.Author, indent(FoldComment(c.Text, it.idx, id)))
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("No comments on %s match\n", id), nil
	}
	if hidden > 0 {
		fmt.Fprintf(&b, "%d hidden: bd comments %s --titles\n", hidden, id)
	}
	return b.String(), nil
}

// Title is a comment's one-line title: its title line (first non-empty line),
// cut to commentHead runes.
func Title(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return cutRunes(strings.TrimLeft(l, "# "), commentHead)
		}
	}
	return ""
}
