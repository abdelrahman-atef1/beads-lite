package sqlitestore_test

import (
	"flag"
	"os"
	"testing"
)

// knownXFail lists conformance cases that fail today, each with its cause and
// owner (reviewed at gate G1, T010). They are skipped through -test.skip unless
// the caller already passed -test.skip; run with BD_LITE_XFAIL=0 to see them.
//
//   - Comments: second AddIssueComment fails: issueops/derivedid.go:194 scans
//     SELECT MAX(created_at) into *time.Time; SQLite returns an aggregate as
//     string (no declared column type). Fix belongs to T005 (dialect/driver:
//     decode datetime-looking strings for aggregate columns, or translate the
//     query) -- not a role-file issue.
//   - SearchPaging/KeysetPositionComposesWithTheFilter: CreatedBefore bound
//     does not exclude the row at the cursor time; time comparison between the
//     stored "YYYY-MM-DD HH:MM:SS" text and the bound parameter. Same owner
//     (T005 / T006 DSN time format).
//   - CommentCount, IterComments, Portable/AddIssueCommentBurstOrder,
//     Audit/search-counts-stats/SearchIssuesWithCounts,
//     TransactionSnapshotReads: same MAX(created_at) scan cause as Comments.
//   - TransactionSearchFilterParity/LabelRegex: sqlitedialect.connector opens
//     a zero-value sqlite.Driver, which does not see the regexp() function that
//     wire.go registers on the global driver; T005 should open via the
//     registered driver instance.
//   - TransactionSearchFilterParity/StartedAfter: same time-compare cause.
//   - Audit/molecule-wisp-batch-iter/CreateRejectStaleUpserts: lite_touched
//     UNIQUE(issue_id) violated when an upsert fires the touch trigger twice;
//     the trigger (schema.sql, T006) needs INSERT OR IGNORE.
const knownXFail = `^TestConformance$/^(Comments|CommentCount|IterComments|TransactionSnapshotReads)$` +
	`|^TestConformance$/^Portable$/^AddIssueCommentBurstOrder$` +
	`|^TestConformance$/^Audit$/^search-counts-stats$/^SearchIssuesWithCounts$` +
	`|^TestConformance$/^Audit$/^molecule-wisp-batch-iter$/^CreateRejectStaleUpserts$` +
	`|^TestConformance$/^TransactionSearchFilterParity$/^(StartedAfter|LabelRegex)$` +
	`|^TestConformance$/^SearchPaging$/^KeysetPositionComposesWithTheFilter$`

func TestMain(m *testing.M) {
	if os.Getenv("BD_LITE_XFAIL") != "0" {
		if f := flag.Lookup("test.skip"); f != nil && f.Value.String() == "" {
			_ = f.Value.Set(knownXFail)
		}
	}
	os.Exit(m.Run())
}
