package compact

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/beads/internal/types"
)

func TestShowBoundedFor246KB(t *testing.T) {
	big := strings.Repeat("مرحبا hello ", 20000) // ~246 KB, multibyte
	is := &types.Issue{ID: "x-1", Title: "t", Description: big, Design: big, Notes: big, Status: "open", CreatedAt: time.Now()}
	var cs []*types.Comment
	for i := 0; i < 200; i++ {
		cs = append(cs, &types.Comment{Author: "a", Text: big, CreatedAt: time.Now()})
	}
	out := Show(is, cs, Options{})
	if len(out) > MaxShowBytes {
		t.Fatalf("show %d bytes > %d", len(out), MaxShowBytes)
	}
	if !strings.Contains(out, "197 older, bd comments x-1 --titles") {
		t.Fatalf("missing hint:\n%s", out)
	}
	if !strings.HasPrefix(out, "x-1 [P0 ") {
		t.Fatalf("header: %q", out[:30])
	}
}

func TestShowSmallUntouched(t *testing.T) {
	out := Show(&types.Issue{ID: "x-2", Title: "hi", Description: "short"}, nil, Options{})
	if !strings.Contains(out, "short") || strings.Contains(out, "--full") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestClampRuneSafe(t *testing.T) {
	s := clamp(strings.Repeat("ع", 10000), 101, "...")
	if len(s) > 101 || !strings.HasSuffix(s, "...") {
		t.Fatalf("len %d", len(s))
	}
	if strings.ContainsRune(s, '�') {
		t.Fatal("split rune")
	}
}

func TestListLimit(t *testing.T) {
	var is []*types.Issue
	for i := 0; i < 120; i++ {
		is = append(is, &types.Issue{ID: fmt.Sprint("x-", i), Title: "t", Status: "open"})
	}
	out := List(is, 500)
	if lines := strings.Count(out, "\n"); lines != MaxListLines {
		t.Fatalf("lines %d", lines)
	}
	if !strings.Contains(out, "... 451 more, use --limit 0") {
		t.Fatalf("hint: %q", out[len(out)-60:])
	}
	if got := List(is[:3], 3); strings.Contains(got, "more") || strings.Count(got, "\n") != 3 {
		t.Fatalf("small: %q", got)
	}
	if got := List(is[:50], 50); strings.Count(got, "\n") != 50 || strings.Contains(got, "more") {
		t.Fatalf("exactly 50: %q", got)
	}
}

func TestSelectFields(t *testing.T) {
	got, err := SelectFields([]byte(`[{"id":"a","title":"t","x":1},{"id":"b","y":2}]`), ParseFields("id, title"))
	if err != nil || string(got) != `[{"id":"a","title":"t"},{"id":"b"}]` {
		t.Fatalf("%s %v", got, err)
	}
	got, _ = SelectFields([]byte(`{"id":"a","z":1}`), []string{"z"})
	if string(got) != `{"z":1}` {
		t.Fatalf("%s", got)
	}
	same, _ := SelectFields([]byte(`{"a":1}`), nil)
	if string(same) != `{"a":1}` {
		t.Fatal("nil fields must pass through")
	}
	if _, err := SelectFields([]byte(`nope`), []string{"a"}); err == nil {
		t.Fatal("want error")
	}
}

func TestShowFoldedAndSection(t *testing.T) {
	c := &types.Comment{Author: "qa", CreatedAt: time.Now(), Text: "Verdict: pass\n\nShip it.\n\n## Evidence\nsecret-evidence-line\nsecond\n\n## Next\nmerge"}
	is := &types.Issue{ID: "x-3", Title: "t", Status: "open"}
	out := Show(is, []*types.Comment{c}, Options{})
	for _, want := range []string{"Verdict: pass", "Ship it.", "[+] Evidence (2 lines)", "[+] Next (1 lines)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret-evidence-line") {
		t.Fatalf("section text leaked:\n%s", out)
	}
	out = Show(is, []*types.Comment{c}, Options{Sections: []string{"evid"}})
	if !strings.Contains(out, "secret-evidence-line") || !strings.Contains(out, "[+] Next") {
		t.Fatalf("--section evid:\n%s", out)
	}
}

func TestShowFlatUnchangedUpTo15Lines(t *testing.T) {
	flat := strings.TrimSuffix(strings.Repeat("a long flat line of old text that is not short at all\n", 15), "\n")
	out := Show(&types.Issue{ID: "x-4", Description: flat}, nil, Options{})
	if strings.Count(out, "a long flat line") != 15 || strings.Contains(out, "more lines") {
		t.Fatalf("flat cut too early:\n%s", out)
	}
	out = Show(&types.Issue{ID: "x-4", Description: flat + "\nextra"}, nil, Options{})
	if !strings.Contains(out, "... 1 more lines (--full)") {
		t.Fatalf("no hint:\n%s", out)
	}
}
