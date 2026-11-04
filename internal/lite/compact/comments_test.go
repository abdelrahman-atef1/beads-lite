package compact

import (
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

func cm(author, text string, d int) *types.Comment {
	return &types.Comment{Author: author, Text: text, CreatedAt: time.Date(2026, 11, d, 10, 0, 0, 0, time.UTC)}
}

func sample() []*types.Comment {
	long := "Head line of a long comment.\n" + strings.Repeat("filler words here ", 30) + "\nVerdict: all good. Next: qa. — swe"
	sect := "Title of sectioned\nlead\n## Result\nline a\nline b\n## Other\nx"
	return []*types.Comment{cm("cto", "short one", 1), cm("swe", long, 2), cm("swe", sect, 3), cm("qa", "last one", 4)}
}

func TestFoldCommentWhole(t *testing.T) {
	s := strings.Repeat("a", 200)
	if got := FoldComment(s, 1, "x-1"); got != s {
		t.Fatalf("200 runes must print whole: %q", got)
	}
}

func TestFoldCommentLongHeadTail(t *testing.T) {
	c := sample()[1].Text
	got := FoldComment(c, 7, "x-1")
	for _, want := range []string{"Head line of a long comment.", "bd comments x-1 --n 7", "3 lines", "Verdict: all good. Next: qa. — swe"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "\n") && strings.Count(got, "\n") != 1 {
		t.Fatalf("expected head line + tail line: %q", got)
	}
}

func TestFoldCommentLongSingleLine(t *testing.T) {
	c := strings.Repeat("word ", 100) + "END-VERDICT"
	got := FoldComment(c, 2, "x-2")
	if !strings.Contains(got, "1 lines") || !strings.HasSuffix(got, "END-VERDICT") || len([]rune(got)) > 330 {
		t.Fatalf("%q", got)
	}
}

func TestFoldCommentSectioned(t *testing.T) {
	got := FoldComment(sample()[2].Text, 3, "x-1")
	if !strings.Contains(got, "[+] Result (2 lines)") || strings.Contains(got, "line a") {
		t.Fatalf("%q", got)
	}
}

func TestShowIndexesAndLast3(t *testing.T) {
	out := Show(&types.Issue{ID: "x-1", Title: "t"}, sample(), Options{})
	if strings.Contains(out, "#1 ") || !strings.Contains(out, "- #2 2026-11-02 swe:") || !strings.Contains(out, "(1 older, bd comments x-1 --titles)") {
		t.Fatalf("%s", out)
	}
}

func TestQueryN(t *testing.T) {
	cs := sample()
	out, err := Comments("x-1", cs, Query{N: 2})
	if err != nil || !strings.Contains(out, cs[1].Text) {
		t.Fatalf("--n 2 must print exact text: %v %q", err, out)
	}
	if _, err := Comments("x-1", cs, Query{N: 9}); err == nil {
		t.Fatal("want error for missing #9")
	}
}

func TestQueryTitles(t *testing.T) {
	out, _ := Comments("x-1", sample(), Query{Titles: true})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 || lines[0] != "#1 2026-11-01 cto: short one" || !strings.HasPrefix(lines[2], "#3 2026-11-03 swe: Title of sectioned") {
		t.Fatalf("%q", out)
	}
}

func TestQueryLast(t *testing.T) {
	out, _ := Comments("x-1", sample(), Query{Last: 2})
	if !strings.Contains(out, "#3 ") || !strings.Contains(out, "#4 ") || strings.Contains(out, "#2 ") ||
		!strings.Contains(out, "2 hidden: bd comments x-1 --titles") || !strings.Contains(out, "[+] Result") {
		t.Fatalf("%q", out)
	}
}

func TestQuerySection(t *testing.T) {
	out, _ := Comments("x-1", sample(), Query{Sections: []string{"result"}})
	if !strings.Contains(out, "## Result") || !strings.Contains(out, "line b") || strings.Contains(out, "short one") || strings.Contains(out, "x\n") {
		t.Fatalf("%q", out)
	}
}

func TestQuerySinceAuthor(t *testing.T) {
	out, _ := Comments("x-1", sample(), Query{Since: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC), Titles: true})
	if strings.Contains(out, "#1 ") || strings.Contains(out, "#2 ") || !strings.Contains(out, "#3 ") {
		t.Fatalf("since: %q", out)
	}
	out, _ = Comments("x-1", sample(), Query{Author: "SWE", Titles: true})
	if strings.Count(out, "\n") != 2 || strings.Contains(out, "cto") || strings.Contains(out, "qa") || !strings.Contains(out, "#2 ") {
		t.Fatalf("author keeps original numbers: %q", out)
	}
	out, _ = Comments("x-1", sample(), Query{Author: "nobody"})
	if !strings.Contains(out, "No comments on x-1 match") {
		t.Fatalf("%q", out)
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Time{"2026-11-01": time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), "12h": now.Add(-12 * time.Hour), "3d": now.AddDate(0, 0, -3), "1w": now.AddDate(0, 0, -7)} {
		got, err := ParseSince(in, now)
		if err != nil || !got.Equal(want) {
			t.Fatalf("%s: %v %v", in, got, err)
		}
	}
	if _, err := ParseSince("soon", now); err == nil {
		t.Fatal("want error")
	}
	if (Query{}).Active() || !(Query{Last: 1}).Active() {
		t.Fatal("Active")
	}
}

func TestSectionedCommentOpensVerdictAndNext(t *testing.T) {
	txt := "Ruling.\n## Policy\nA\nB\n## Verdict\nFAIL on item three\n## Next\nswe fixes item three"
	got := FoldComment(txt, 4, "x-1")
	for _, want := range []string{"FAIL on item three", "swe fixes item three", "[+] Policy (2 lines)", `bd comments x-1 --n 4 --section "Policy"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(FoldComment("T\n## Verdict\nok", 1, "x-1"), "(open:") {
		t.Fatal("no hint when nothing is folded")
	}
}

func TestShowSectionNoMatchSaysSo(t *testing.T) {
	is := &types.Issue{ID: "x-1", Title: "t", Description: "T\n## Evidence\na"}
	if out := Show(is, nil, Options{Sections: []string{"zzzz"}}); !strings.Contains(out, `No section matches "zzzz"`) {
		t.Fatalf("%s", out)
	}
	if out := Show(is, nil, Options{Sections: []string{"evid"}}); strings.Contains(out, "No section matches") || !strings.Contains(out, "## Evidence") {
		t.Fatalf("%s", out)
	}
	out, _ := Comments("x-1", sample(), Query{Sections: []string{"zzzz"}})
	if !strings.Contains(out, "No section matching") {
		t.Fatalf("%s", out)
	}
}
