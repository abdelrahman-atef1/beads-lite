// Package foldtext splits bead text written in the "folded text" format
// (beads-lite-1bo5.6.46): a plain title line, an optional short lead, then
// "## Name" sections. Text with no section line is "flat" and is left whole.
// Pure text split: no storage, no schema.
package foldtext

import "strings"

// Section is one "## Name" block. Body excludes the heading line.
type Section struct {
	Name string
	Body string
}

// Doc is the parsed form of one text field or comment.
type Doc struct {
	Title    string // first non-empty line (empty if the text starts with a section)
	Lead     string // lines between the title and the first section
	Sections []Section
	Flat     bool // true when there is no "## " line outside a code fence
}

// Parse splits text. A section starts at a line beginning with "## " that is
// outside a fenced code block; "###" and deeper stay inside their section.
func Parse(text string) Doc {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var d Doc
	var head []string
	cur := -1
	fence := ""
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if f := fenceMarker(t); f != "" {
			if fence == "" {
				fence = f
			} else if fence == f {
				fence = ""
			}
		}
		if fence == "" && strings.HasPrefix(ln, "## ") {
			d.Sections = append(d.Sections, Section{Name: strings.TrimSpace(ln[3:])})
			cur = len(d.Sections) - 1
			continue
		}
		if cur < 0 {
			head = append(head, ln)
		} else {
			d.Sections[cur].Body += ln + "\n"
		}
	}
	d.Flat = len(d.Sections) == 0
	for i := range d.Sections {
		d.Sections[i].Body = strings.Trim(d.Sections[i].Body, "\n")
	}
	h := strings.TrimSpace(strings.Join(head, "\n"))
	if d.Flat {
		d.Title = firstLine(h)
		return d
	}
	if i := strings.IndexByte(h, '\n'); i >= 0 {
		d.Title, d.Lead = strings.TrimSpace(h[:i]), strings.TrimSpace(h[i+1:])
	} else {
		d.Title = h
	}
	return d
}

// Lines counts the non-empty lines of s.
func Lines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// Match reports whether the section name contains any of want
// (case-insensitive substring). An empty want matches nothing.
func (s Section) Match(want []string) bool {
	n := strings.ToLower(s.Name)
	for _, w := range want {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" && strings.Contains(n, w) {
			return true
		}
	}
	return false
}

func fenceMarker(t string) string {
	switch {
	case strings.HasPrefix(t, "```"):
		return "```"
	case strings.HasPrefix(t, "~~~"):
		return "~~~"
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
