# ADR 0002: Upstream merge policy for the beads-lite fork

- Status: accepted (CTO, 2026-10-26). Plan task T043 of `specs/001-fast-compact-tracker`.
- Governs: every change to files that came from upstream bd (`gastownhall/beads`), and every decision to take upstream code.

## Context

beads-lite is a fork of a 359k-line Go codebase that moves fast (138 storage commits in 10 weeks, measured for the plan; release candidates `v1.3.2-rc.*` already exist). Our branch `lite` sits on upstream tag v1.3.1 (`c1c4b642a`) plus our own files. We do not have the people to chase upstream, and the product is one owner's tracker, not a public project. What we need from upstream is a stable, known CLI/JSON surface (the compatibility rows in `specs/001-fast-compact-tracker/contracts/cli-compat.md`), not its roadmap.

Measured baseline at this ADR: `git diff --shortstat c1c4b642a lite` = 36 files changed, 5,390 insertions, 0 deletions (added files only: specs, `.specify`, `.claude`, notes). Our code has not yet touched an upstream file. This is the best moment to fix the rules.

## Decision

### 1. Pin

- The base is **upstream tag v1.3.1**. Remote `upstream` stays configured; we fetch tags only when a merge decision is open (section 2). We never track upstream `main`, never rebase `lite` onto it, never merge a branch, only a **tag**.
- `lite` is never force-pushed after another agent has built on it (the one rewrite on 2026-10-25 was the exception, see `SETUP-NOTES.md`).
- Upstream licences stay in the tree unchanged (bd: MIT, `LICENSE`; vendored third-party in `THIRD_PARTY_LICENSES`).

### 2. Merge only by decision

Nothing from upstream enters `lite` unless the CTO records a merge decision in `docs/decisions/upstream-merges.md` (one dated paragraph: tag, why, what we skip, the diff-size numbers from section 4 before and after, test result). Reasons that justify a decision:

1. A security fix in code we ship (checked against the tag's changelog and the files we link).
2. A bug that bites one of the migrated corpora and that we cannot cheaply fix in our own folders.
3. A CLI/JSON surface change that the owner's tools (rayeh gates, board, company scripts) need.
4. The owner asks for a specific upstream feature.

"New release exists" and "we are behind" are not reasons. Review cadence: once per quarter the CTO reads the upstream changelog from our pin to the newest stable tag (rc tags are skipped), writes "no merge" or a merge decision in the log, and that is all. A review that finds nothing is a one-line entry.

How a merge is done, when decided:

- Prefer **cherry-picking the specific fix** over merging a whole tag. Merge a whole tag only when it is a patch release (v1.3.x) with a short list; a minor (v1.4+) needs its own ADR first.
- Work on a branch `merge/<tag>` from `lite`, never on `lite` directly. Conflicts are resolved keeping our `// lite:` hunks (section 3) and the file layout.
- Schema is frozen (ADR 0001): upstream migrations after main 0066 / ignored 0026 are adopted only by a new ADR that amends 0001. A merge that adds a migration and has no such ADR is rejected.
- Gate before landing: the two-tier backend conformance harness (G1), the CLI parity corpus run (W6), and the existing upstream tests for the files we touch. Any red = not landed.
- The merge lands on `lite` by the owner-approved path for the project (merge commit, no squash, so `git log upstream-tag..lite` stays meaningful).

### 3. `// lite:` marker rules

Goal: a future merge can find every line we changed in upstream files with one `grep`, and the change is small enough to re-apply by hand.

1. **Our code lives in our folders.** New logic goes in `internal/storage/sqlitedialect`, `internal/storage/sqlitestore`, `internal/lite/*`, `internal/migration/sqlitelite`, `cmd/bd/lite_*.go`, `bench/`, `docs/`, `specs/`. A new file added inside an upstream folder is named `lite_*.go` (or `lite_*_test.go`).
2. **An upstream file is edited only for one of two reasons:** wiring (a call into our folders; owner of wiring is W5) or making a Dolt-only path safe on SQLite (the 16 `issueops` files listed in task T007). Anything else needs a CTO ruling first.
3. **Every changed hunk in an upstream file carries a marker.** One-line change: a trailing or preceding comment `// lite: <reason in one line>`. Multi-line change: `// lite:begin <reason>` ... `// lite:end`. The reason says what and why ("skip dolt_commit on sqlite"), not who.
4. **Smallest hunk that works.** No reformatting, no renames, no moved functions, no import reordering, no unrelated cleanup, no comment edits in upstream code. Prefer a one-line capability check that calls into our folder over inlining logic. A hunk that grows past ~30 lines is moved into a `lite_*.go` file and the upstream site keeps the call.
5. **Dolt behaviour does not change.** Existing upstream tests for the touched file stay green without edits to the tests. If a test must change, that change is marked too and listed in the PR/commit message.
6. **Deleting upstream code is not allowed** (comment out nothing, remove nothing): add a branch around it. Deletions make merges conflict and hide what upstream did. The exception is `go.mod`/`go.sum` and files we replace wholesale (listed in `docs/decisions/` when it happens).
7. **No marker, no merge.** The reviewer (CTO or QA) rejects any diff to an upstream file that contains a changed line outside a marker, which the check below finds.

Check (run by the author before asking for review, and by the reviewer):

```
# changed lines in upstream files, excluding our folders
git diff --numstat c1c4b642a lite -- . \
  ':!specs' ':!docs' ':!bench' ':!.specify' ':!.claude' ':!SETUP-NOTES.md' \
  ':!internal/lite' ':!internal/migration/sqlitelite' \
  ':!internal/storage/sqlitedialect' ':!internal/storage/sqlitestore' \
  ':!cmd/bd/lite_*' ':!go.mod' ':!go.sum'
# every marker we have
grep -rn "lite:" --include=*.go .
```

A changed line in the first command's output that is not inside a `lite:` marker in the second is a blocking finding.

### 4. Diff-size review

The size of what we changed in upstream files is the cost of every future merge, so it is measured and capped.

- **Metric:** from the first command in section 3, (a) number of upstream files touched, (b) added + deleted lines in them. Our own folders do not count.
- **Initial ceiling:** at most **40 upstream files** and **600 changed lines** (about 16 issueops files at ~25 lines each plus wiring in `cmd/bd`, `cmd/bd/main.go` and the store factory). The ceiling is a starting guess from the plan, to be replaced by the real number at G2.
- **When measured:** at G1 (T010), at G2 (T018), at every merge decision (before and after), and in the quarterly review. The number goes in the bead or log entry, one line.
- **Over the ceiling:** do not land more upstream-file edits. The CTO decides one of: move the logic into a `lite_*` file, raise the ceiling by a short ADR with the reason, or stop editing upstream and wrap the call from our side.
- **Freeze trigger:** if a merge decision's cost (resolving conflicts, re-running G1 and W6) exceeds one engineer-week twice in a row, the fork is frozen at its current tag permanently: no further merges, security fixes cherry-picked by hand only. This is an allowed outcome, not a failure.

## Alternatives rejected

1. **Track upstream `main` and rebase regularly.** Gives free fixes, but upstream is changing storage code (versioned beads, attribution, participation) that we deliberately froze; each rebase would re-run the whole conformance and parity gates and cost a week at the observed pace. Rejected under *Boring technology*: a fork we can leave alone for a year beats one that needs a quarterly rebase.
2. **Unmarked hard fork (edit freely, never merge).** Cheapest today, but it throws away the option to take a security fix or a needed CLI change; after a year nobody can tell which lines are ours. Rejected: the markers cost a comment per hunk and keep the exit open.
3. **Keep our changes as a patch series (quilt/`git format-patch`) applied over each upstream tag.** Clean in theory, but it adds tooling nobody maintains and our changes are mostly new folders, not patches. Rejected: more moving parts than the marker convention for the same result.

## Cost

Local policy and a `grep`: $0 per active user, $0 per idle user, no service, no quota. The real cost is engineer time at a merge decision, capped by the freeze trigger in section 4.

## Exit path

If upstream dies, relicenses, or we stop wanting its code: freeze at the pinned tag (section 4). Nothing depends on an upstream service; the tree is self-contained and the data stays upstream-JSONL (`bd export`, ADR 0001 exit path). If upstream ever drops MIT, we keep building from the last MIT tag we hold.
