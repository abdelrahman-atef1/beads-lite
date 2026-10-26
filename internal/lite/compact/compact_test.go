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
	out := Show(is, cs)
	if len(out) > MaxShowBytes {
		t.Fatalf("show %d bytes > %d", len(out), MaxShowBytes)
	}
	if !strings.Contains(out, "195 older comments hidden, use --full") {
		t.Fatalf("missing hint:\n%s", out)
	}
	if !strings.HasPrefix(out, "x-1 [P0 ") {
		t.Fatalf("header: %q", out[:30])
	}
}

func TestShowSmallUntouched(t *testing.T) {
	out := Show(&types.Issue{ID: "x-2", Title: "hi", Description: "short"}, nil)
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
