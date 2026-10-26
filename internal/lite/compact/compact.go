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

	"github.com/steveyegge/beads/internal/types"
)

const (
	// MaxShowBytes is the hard ceiling for one rendered bead (SC-006).
	MaxShowBytes = 8 * 1024
	// MaxListLines is the hard ceiling for one rendered list, including hint lines.
	MaxListLines = 50

	fieldBytes   = 1200 // per long field (description, design, notes, acceptance)
	commentBytes = 400  // per comment
	maxComments  = 5    // newest comments shown
	titleRunes   = 100  // list-line title width
)

// Show renders one bead as compact text of at most MaxShowBytes bytes.
// Long fields and older comments are cut with a hint naming --full.
func Show(is *types.Issue, comments []*types.Comment) string {
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
	section(&b, "DESCRIPTION", is.Description)
	section(&b, "DESIGN", is.Design)
	section(&b, "ACCEPTANCE", is.AcceptanceCriteria)
	section(&b, "NOTES", is.Notes)

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
			fmt.Fprintf(&b, "- %s %s: %s\n", day(c.CreatedAt), c.Author, cut(c.Text, commentBytes, ""))
		}
	}
	return clamp(b.String(), MaxShowBytes, "\n... (output cut at 8 KB, use --full)\n")
}

func section(b *strings.Builder, name, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	fmt.Fprintf(b, "\n%s\n%s\n", name, cut(strings.TrimSpace(text), fieldBytes, " ... (cut, use --full)"))
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
