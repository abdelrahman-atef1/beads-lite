// Package compact renders bounded, filtered text for agents (beads-lite W14,
// SC-006): `show` at most MaxShowBytes, `list` at most MaxListLines lines, each
// with a "more" hint. --full (upstream text) is the caller's job; this package
// only produces the compact form. JSON is never touched except by SelectFields.
package compact

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/steveyegge/beads/internal/foldtext"
	"github.com/steveyegge/beads/internal/types"
)

const (
	// MaxShowBytes is the hard ceiling for one rendered bead (SC-006).
	MaxShowBytes = 8 * 1024
	// MaxListLines is the hard ceiling for one rendered list, including hint lines.
	MaxListLines = 50

	flatLines   = 15   // flat (section-less) text prints whole up to this many lines
	flatBytes   = 2000 // backstop for very long flat lines
	maxComments = 5    // newest comments shown
	titleRunes  = 100  // list-line title width
)

// Show renders one bead as compact text of at most MaxShowBytes bytes.
// Long fields and older comments are cut with a hint naming --full.
//
// Text written in the folded format (title, lead, "## Name" sections) prints
// its title, lead and one "[+] Name (N lines)" line per section; the sections
// named in opts.Sections print in full. Flat text prints whole up to flatLines
// lines, then a hint.
func Show(is *types.Issue, comments []*types.Comment, opts Options) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s [P%d %s %s] %s\n", is.ID, is.Priority, is.IssueType, is.Status, oneLine(is.Title))
	var meta []string
	if is.Assignee != "" {
		meta = append(meta, "assignee: "+is.Assignee)
	}
	if !is.CreatedAt.IsZero() {
		meta = append(meta, "created: "+day(is.CreatedAt))
	}
	if !is.UpdatedAt.IsZero() {
		meta = append(meta, "updated: "+day(is.UpdatedAt))
	}
	if is.ClosedAt != nil {
		meta = append(meta, "closed: "+day(*is.ClosedAt))
	}
	if len(meta) > 0 {
		b.WriteString(strings.Join(meta, "  ") + "\n")
	}
	if is.CloseReason != "" {
		b.WriteString("close reason: " + cut(is.CloseReason, 300, "") + "\n")
	}
	section(&b, "DESCRIPTION", is.Description, opts)
	section(&b, "DESIGN", is.Design, opts)
	section(&b, "ACCEPTANCE", is.AcceptanceCriteria, opts)
	section(&b, "NOTES", is.Notes, opts)

	if n := len(comments); n > 0 {
		shown := comments
		if n > maxComments {
			shown = comments[n-maxComments:]
		}
		fmt.Fprintf(&b, "\nCOMMENTS (%d)\n", n)
		if n > len(shown) {
			fmt.Fprintf(&b, "(%d older comments hidden, use --full)\n", n-len(shown))
		}
		for _, c := range shown {
			fmt.Fprintf(&b, "- %s %s:\n%s\n", day(c.CreatedAt), c.Author, indent(fold(c.Text, opts)))
		}
	}
	return clamp(b.String(), MaxShowBytes, "\n... (output cut at 8 KB, use --full)\n")
}

// Options selects which folded sections Show prints in full.
type Options struct {
	Sections []string // --section values (substring, case-insensitive)
}

func section(b *strings.Builder, name, text string, opts Options) {
	if strings.TrimSpace(text) == "" {
		return
	}
	fmt.Fprintf(b, "\n%s\n%s\n", name, fold(text, opts))
}

// fold renders one text value (field or comment) in the compact form.
func fold(text string, opts Options) string {
	text = strings.TrimSpace(text)
	d := foldtext.Parse(text)
	if d.Flat {
		lines := strings.Split(text, "\n")
		if len(lines) <= flatLines {
			return cut(text, flatBytes, " ... (cut, use --full)")
		}
		return cut(strings.Join(lines[:flatLines], "\n"), flatBytes, " ...") + fmt.Sprintf("\n... %d more lines (--full)", len(lines)-flatLines)
	}
	var out []string
	if d.Title != "" {
		out = append(out, d.Title)
	}
	if d.Lead != "" {
		out = append(out, d.Lead)
	}
	for _, s := range d.Sections {
		if s.Match(opts.Sections) {
			out = append(out, "## "+s.Name, s.Body)
		} else {
			out = append(out, fmt.Sprintf("[+] %s (%d lines)", s.Name, foldtext.Lines(s.Body)))
		}
	}
	return strings.Join(out, "\n")
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// List renders one line per bead, at most MaxListLines lines in total. total is
// the number of beads that matched (>= len(issues)); a hint line says how many
// are not shown.
func List(issues []*types.Issue, total int) string {
	if total < len(issues) {
		total = len(issues)
	}
	limit := MaxListLines
	if total > limit {
		limit-- // room for the hint line
	}
	shown := issues
	if len(shown) > limit {
		shown = shown[:limit]
	}
	var b strings.Builder
	for _, is := range shown {
		fmt.Fprintf(&b, "%s P%d %s %s", is.ID, is.Priority, is.Status, cutRunes(oneLine(is.Title), titleRunes))
		if is.Assignee != "" {
			b.WriteString(" @" + is.Assignee)
		}
		b.WriteByte('\n')
	}
	if more := total - len(shown); more > 0 {
		fmt.Fprintf(&b, "... %d more, use --limit 0 or --full\n", more)
	}
	return b.String()
}

// SelectFields keeps only the named top-level keys of a JSON object or of each
// object in a JSON array. Unknown names are ignored. An empty fields list
// returns data unchanged.
func SelectFields(data []byte, fields []string) ([]byte, error) {
	if len(fields) == 0 {
		return data, nil
	}
	want := make(map[string]bool, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			want[f] = true
		}
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("--fields: input is not JSON: %w", err)
	}
	return json.Marshal(filter(v, want))
}

func filter(v any, want map[string]bool) any {
	switch t := v.(type) {
	case []any:
		for i := range t {
			t[i] = filter(t[i], want)
		}
		return t
	case map[string]any:
		out := make(map[string]any, len(want))
		for k, x := range t {
			if want[k] {
				out[k] = x
			}
		}
		return out
	}
	return v
}

// ParseFields splits a --fields value "a,b,c".
func ParseFields(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func day(t time.Time) string { return t.UTC().Format("2006-01-02") }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "..."
}

// cut shortens s to at most n bytes on a rune boundary, appending suffix.
func cut(s string, n int, suffix string) string {
	if len(s) <= n {
		return s
	}
	end := n
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + suffix
}

// clamp guarantees len(result) <= max, including the suffix.
func clamp(s string, max int, suffix string) string {
	if len(s) <= max {
		return s
	}
	return cut(s, max-len(suffix), "") + suffix
}
