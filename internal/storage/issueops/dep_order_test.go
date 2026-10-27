package issueops

import (
	"testing"

	"github.com/steveyegge/beads/internal/storage/depid"
	"github.com/steveyegge/beads/internal/types"
)

// T046: the list/ready counts query aggregates edges in scan order; they must
// come out in primary-key order like upstream.
func TestSortDepsByPK(t *testing.T) {
	var deps []*types.Dependency
	for _, id := range []string{"q-a", "q-b", "q-c", "q-d", "q-e", "q-f"} {
		deps = append(deps, &types.Dependency{IssueID: "q-src", DependsOnID: id})
	}
	sortDepsByPK(deps)
	for i := 1; i < len(deps); i++ {
		if depid.New("q-src", deps[i-1].DependsOnID) > depid.New("q-src", deps[i].DependsOnID) {
			t.Fatalf("not in primary-key order at %d", i)
		}
	}
	if deps[0].DependsOnID == "q-a" && deps[1].DependsOnID == "q-b" && deps[2].DependsOnID == "q-c" {
		t.Fatal("still alphabetical")
	}
}
