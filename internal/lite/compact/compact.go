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
	maxComments = 3    // newest comments shown
	titleRunes  = 100  // list-line title width

	commentWhole = 200 // flat comments up to this many runes print whole
	commentHead  = 80  // longer ones: first line cut to this
	commentTail  = 140 // ... plus this many trailing runes
)

// Show renders one bead as compact text of at most MaxShowBytes bytes.
// Long fields and older comments are cut with a hint naming --full.
//
// Text written in the folded format (title, lead, "## Name" sections) prints
// its title, lead and one "[+] Name (N lines)" line per section; the sections
// named in opts.Sections print in full. Flat text prints whole up to flatLines
// lines, then a hint.
func Show(is *types.Issue, comments []*types.Comment, opts Options) string {
	hits := 0
	if len(opts.Sections) > 0 {
		opts.hits = &hits
	}
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
			fmt.Fprintf(&b, "(%d older, bd comments %s --titles)\n", n-len(shown), is.ID)
		}
		for i, c := range shown {
			idx := n - len(shown) + i + 1
			fmt.Fprintf(&b, "- #%d %s %s:\n%s\n", idx, day(c.CreatedAt), c.Author, indent(FoldComment(c.Text, idx, is.ID, opts)))
		}
	}
	if len(opts.Sections) > 0 && hits == 0 {
		fmt.Fprintf(&b, "\nNo section matches %q (sections are the '## Name' headings; see the [+] lines above)\n", strings.Join(opts.Sections, ", "))
	}
	return clamp(b.String(), MaxShowBytes, "\n... (output cut at 8 KB, use --full)\n")
}

// Options selects which folded sections Show prints in full.
type Options struct {
	Sections []string // --section values (substring, case-insensitive)
	hits     *int     // counts sections printed because of Sections
}

func (o Options) hit() {
	if o.hits != nil {
		*o.hits++
	}
}

func section(b *strings.Builder, name, text string, opts Options) {
	if strings.TrimSpace(text) == "" {
		return
	}
	fmt.Fprintf(b, "\n%s\n%s\n", name, fold(text, opts))
}

// FoldComment renders one comment in the compact form (the one fold used by
// show and by the comments flags). Sectioned text (## Name) prints title, lead
// and section names. Flat text of at most commentWhole runes prints whole;
// longer flat text prints its first line (cut), a marker with line and char
// counts and the command that prints the whole comment, then the last
// commentTail runes (where the verdict and signature sit).
// idx is the 1-based comment number (oldest = 1).
func FoldComment(text string, idx int, id string, opts ...Options) string {
	text = strings.TrimSpace(text)
	var o Options
	if len(opts) > 0 {
		o = opts[0]
	}
	if d := foldtext.Parse(text); !d.Flat {
		return foldSectionedComment(d, idx, id, o)
	}
	rs := []rune(text)
	if len(rs) <= commentWhole {
		return text
	}
	head := ""
	for _, l := range strings.Split(text, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			head = cutRunes(l, commentHead)
			break
		}
	}
	tail := strings.Join(strings.Fields(string(rs[len(rs)-commentTail:])), " ")
	return fmt.Sprintf("%s [... %d lines, %d ch; bd comments %s --n %d]\n... %s",
		head, strings.Count(text, "\n")+1, len(rs), id, idx, tail)
}

// openSections are the section names a sectioned comment prints in full in the
// default view: the reader needs the verdict and the next step without a second
// command. Each is capped at openLines lines; the rest is folded with a hint.
var openSections = []string{"verdict", "next"}

const openLines = 8

// foldSectionedComment: title, lead, Verdict and Next in full (capped), other
// sections as "[+] Name (N lines)", then one line with the exact command that
// opens a folded section.
func foldSectionedComment(d foldtext.Doc, idx int, id string, opts Options) string {
	var out []string
	if d.Title != "" {
		out = append(out, d.Title)
	}
	if d.Lead != "" {
		out = append(out, d.Lead)
	}
	first := ""
	for _, s := range d.Sections {
		switch {
		case s.Match(opts.Sections):
			opts.hit()
			out = append(out, "## "+s.Name, s.Body)
		case s.Match(openSections):
			lines := strings.Split(s.Body, "\n")
			body := s.Body
			if len(lines) > openLines {
				body = strings.Join(lines[:openLines], "\n") + fmt.Sprintf("\n... %d more lines", len(lines)-openLines)
			}
			out = append(out, "## "+s.Name, body)
		default:
			if first == "" {
				first = s.Name
			}
			out = append(out, fmt.Sprintf("[+] %s (%d lines)", s.Name, foldtext.Lines(s.Body)))
		}
	}
	if first != "" {
		out = append(out, fmt.Sprintf("(open: bd comments %s --n %d --section %q)", id, idx, first))
	}
	return strings.Join(out, "\n")
}

// fold renders one text value (field) in the compact form.
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
			opts.hit()
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
