package foldtext

import "testing"

func TestParseSections(t *testing.T) {
	d := Parse("Verdict: pass\n\nShip it.\n\n## Evidence\n- a\n- b\n### sub\nx\n\n## Next\ndo it\n")
	if d.Flat || d.Title != "Verdict: pass" || d.Lead != "Ship it." || len(d.Sections) != 2 {
		t.Fatalf("%+v", d)
	}
	if d.Sections[0].Name != "Evidence" || Lines(d.Sections[0].Body) != 4 {
		t.Fatalf("%+v", d.Sections[0])
	}
	if !d.Sections[1].Match([]string{"ne"}) || d.Sections[1].Match([]string{"evid"}) {
		t.Fatal("match")
	}
}

func TestParseFenceAndFlat(t *testing.T) {
	d := Parse("Title\n```\n## not a section\n```\nmore")
	if !d.Flat || d.Title != "Title" {
		t.Fatalf("%+v", d)
	}
	if d := Parse("one\ntwo\n### deep"); !d.Flat {
		t.Fatal("### must not split")
	}
	if d := Parse("## Only\nbody"); d.Title != "" || len(d.Sections) != 1 {
		t.Fatalf("%+v", d)
	}
}
