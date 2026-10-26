# ADR 0004: Gate G1 review (conformance green, empty XFAIL list)

- Status: accepted with one required fix (CTO, 2026-10-26). Plan task T010 of `specs/001-fast-compact-tracker`. The task text says `0003-g1-xfail-review.md`; number 0003 is taken by the migration-extract ADR on `cto/extract`, so this is the next free number.
- Reviewed: branch `swe/g1-fixes` at `d5a321356` (worktree `E:/Projects/beads-lite-g1`), diff `a67e6e06c..d5a321356` (10 files, +181/-74).

## Verdict

**G1 passes, conditional on one small fix before `swe/g1-fixes` lands on `lite`:**

1. **Blocking (ADR 0002 section 3, rules 3 and 7):** the two changed lines in the upstream file `internal/storage/sqlbuild/filter.go` (`args = append(args, *tc.v)` and the `Overdue` bind) carry no `// lite:` marker. Add a one-line `// lite: bind time.Time, not an RFC3339 string; SQLite compares text columns, see sqlitestore/dsn.go` on each (or a `lite:begin/end` pair). No logic change. No marker, no merge.
2. Non-blocking: re-run the conformance suite once on a quiet machine (see "Test run" below) and make the harness close the store before TempDir cleanup.

## XFAIL list

`knownXFail` in `internal/storage/sqlitestore/xfail_test.go` is now the empty string and `TestMain` skips the list when empty. The 11 cases of the earlier list (Comments, CommentCount, IterComments, TransactionSnapshotReads, AddIssueCommentBurstOrder, SearchIssuesWithCounts, CreateRejectStaleUpserts, StartedAfter, LabelRegex, KeysetPositionComposesWithTheFilter) now run and none failed on an assertion. An empty list cannot touch a row of `contracts/cli-compat.md`; the condition "no XFAIL case touches a compat row" holds trivially. Rule kept: a future XFAIL entry carries cause and owner and is reviewed here again.

## Test run (this review, Go 1.26.5, CGO=0, busy PC)

- `sqlitedialect`, `sqlbuild`, `issueops` packages: pass.
- `sqlitestore` `TestConformance`: run twice. **Zero assertion failures.** Each run reported 4 to 6 subtests failed only with `TempDir RemoveAll cleanup: ... .beads: The directory is not empty`, a different set of tests each time (LabelIdempotent, CommentCount, Slots, ...). That is a Windows file-handle race (WAL/shm file still held, antivirus), not a store logic result, but it makes "green" noisy. Required follow-up (swe, non-blocking): call the store's `Close` in `t.Cleanup` before the TempDir removal, and use a retrying remove in the harness (company lesson: `fs.rmSync`/RemoveAll on Windows). Whoever runs the gate in CI/QA should see zero failures on a quiet machine.
- Dolt-side tests (`cgo` tag) cannot be built here (`CGO_ENABLED=0`, no C compiler); no upstream `_test.go` file was edited by G1 (`git diff c1c4b642a HEAD -- '*_test.go'` shows only our folders), so ADR 0002 rule 5 is met for the tests that exist.

## Review of the three requested changes

### 1. `sqlbuild/filter.go`: time.Time binds (shared upstream code)

Change: the 12 `CreatedAfter/Before ... DueAfter/Before` filters and `Overdue` bound an RFC3339 string; now they bind `time.Time`.

- **Dolt regression risk: low.** The same file already binds `AfterCreatedAt` as `time.Time` on Dolt (comment at the keyset block: "the same binding EventsSince uses"), so the Dolt drivers (go-sql-driver DSN with `parseTime=true`, location UTC by default, and `dolthub/driver` embedded) already accept this path. go-sql-driver converts a `time.Time` into the connection location (UTC) before formatting, so a zoned filter value is now compared correctly; the old string carried an offset (`+02:00`) whose handling by Dolt's literal parser was never tested. Residual difference: the driver sends microseconds, the old string was truncated to seconds; this changes results only for a sub-second bound against a `datetime(0)` column, an edge no corpus has.
- **Not verified here:** the Dolt `cgo` test tier (no toolchain on this machine). QA must run `internal/workapi` golden tests (`list_filter_golden.json`) and the Dolt filter tests under cgo before G2. If any fail, the fix is in our folder (format on the SQLite side), never a revert in `filter.go` that diverges again.
- **Merge cost (ADR 0002 section 4): small.** 2 changed lines, one function. Conflict resolution is re-applying two lines. Only problem is the missing marker (blocking item 1). Alternative rejected: converting the string inside `sqlitedialect.Translate`/the driver (parsing RFC3339 out of arbitrary bound strings is fragile and would also change strings that are meant to be strings). Alternative considered: leave `filter.go` alone and add a bind hook in our driver for `*time.Time`-formatted strings; rejected for the same reason.
- **Diff-size at G1 (section 4 metric):** 11 upstream files touched (`issueops/*` x10, `sqlbuild/filter.go`) plus `NOTES-T008.md` at the root, about 80 changed lines. Ceiling 40 files / 600 lines: **well under**. Two hunks edit existing lines rather than only adding (`journal_prune.go`, `wisp_routing.go`, now `filter.go`); rule 6 forbids deletion, these are in-place replacements of a call by a wrapped call, accepted as the smallest hunk that works. `NOTES-T008.md` should move to `docs/` (non-blocking).

### 2. `sqlitedialect/rows.go`: datetime decode of expression columns

Mechanism: for columns whose declared type is empty (aggregates and other expressions) and whose text is exactly `YYYY-MM-DD HH:MM:SS`, return `time.Time` (UTC), matching what Dolt's driver returns for `MAX(created_at)` and what `derivedid.go` scans into `*time.Time`. Plain columns are untouched. Lives entirely in our folder; zero merge cost.

- Correct and minimal; the wrapper only attaches when an expression column exists, per-row cost is one length check per such column.
- Known edge, non-blocking: an expression that yields a datetime-looking string (`COALESCE(created_at,'')`, `MAX(title)` of a title that is a timestamp) and is scanned into a `string` comes back as RFC3339 (`T`...`Z`), not the stored text. Dolt behaves the same for true datetime expressions, so parity holds there; for string expressions it would differ. swe should add one unit test that pins this behaviour (scan of `COALESCE(created_at,'')` into string) so a change is visible. No call site in the conformance suite hits it.
- Registered-driver change in `driver.go` (open on the process-wide modernc driver so `regexp()` is visible) is correct; note that user functions are therefore process-global, which is fine for a single-store CLI.

### 3. `lite_touched` trigger (`sqlitestore/schema.sql`)

`INSERT OR IGNORE ... VALUES` became `INSERT ... SELECT NEW.id WHERE NOT EXISTS (...)`. Same set semantics (one row per touched issue), no UNIQUE violation when an upsert fires the trigger twice in a statement. Inside our folder.

- ADR 0001 (frozen schema) is not violated in substance: `lite_touched` is our own table, its columns are unchanged, only trigger bodies changed. Because the file uses `CREATE TRIGGER IF NOT EXISTS`, a database created with the earlier trigger text keeps the old trigger. **No such database exists outside tests** (T011, `bd init` default, has not landed), so this is free now. After T011 lands, any further trigger or schema-text change needs a numbered migration step and a note under ADR 0001.
- The `_timezone=UTC` DSN addition is right: it makes `time.Time` binds independent of the local zone, which is exactly what the filter change relies on. Required together with `_time_format=datetime`; keep the two in one comment (already done).

## Cost

Per active user: $0 (local files). Per idle user: $0. No service, no quota. No new dependency.

## Consequence

- swe: add the `// lite:` markers (blocking), the Close-before-cleanup fix, the string-expression pin test, and move `NOTES-T008.md` under `docs/`.
- qa: run the cgo Dolt tier and the workapi golden tests against this branch before G2 (T018).
- T010 may be ticked once item 1 is in.

## Related finding (T020, whisper-flow: 60 of 96 beads get an extra history version)

Asked by CoS; evidence is code reading of `swe/t020` (`internal/migration/sqlitelite/load.go` `loadVersions`, `extract.go` `diffSQL`/`buildBeads`), not a re-run on the corpus.

- **The policy is correct:** the last version in `lite_issue_versions` must equal the exported row, so `History()` ends at the current state. When the committed Dolt state differs from the export (uncommitted working-set changes, shown by the `dolt_status` count), appending a final version is right.
- **The rate is a bug signal, not truth.** 60 of 96 beads edited since their last commit is implausible. Cause (high confidence): `diffSQL` selects only 18 `to_*` columns (no `metadata`, `external_ref`, `due_at`, `defer_until`, `started_at`, `labels`-adjacent scalars, ephemeral flags, and so on), and `buildBeads` rebuilds a `types.Issue` from them, while `sameVersion` compares that against the full exported row. Any bead with one non-empty column outside the 18 (or a time rendered differently by `parseDolt` and `normTime`) looks "changed". It also means every older snapshot silently lacks those columns.
- **Required fix (swe, T020, before G2):** (a) select every column of `issues` from `dolt_diff_issues` (`d.to_*`), so snapshots are full; (b) compare only on columns present on both sides and normalise times the same way; (c) make `ExtraVersion` report the first differing column per bead so the count is explainable; (d) after the fix, expected extras should be near the `dolt_status` dirty count only. Until then do not accept T020's verify numbers as "faithful history".
