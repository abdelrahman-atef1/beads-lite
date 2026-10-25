# ADR 0001: Frozen SQLite schema for beads-lite

- Status: accepted (CTO, 2026-10-25). Plan task P0.3 of `specs/001-fast-compact-tracker`.
- Owners of the code it governs: W2 (`internal/storage/sqlitestore/schema.sql`, `schema.go`), W8 (migration), W10 (history).

## Context

Upstream bd builds its schema by running migrations at every open: at v1.3.1 (`c1c4b642a`, the version installed on the owner's PC) that is 66 main migrations (`internal/storage/schema/migrations/0001-0066`) plus 26 "ignored" ones (`migrations/ignored/0001-0026`, the wisp and journal tables that Dolt keeps out of version control). A fresh `bd init` on this PC took 5 min 57 s (measured 2026-10-25, scratch project, metrics off), mostly this migration run on embedded Dolt.

beads-lite does not inherit that machinery. It ships **one frozen schema** that equals the logical v1.3.1 schema, translated to SQLite, plus three tables of its own. `upstream main` (after v1.3.1) has migrations 0067-0072 (`issue_versions` "versioned beads", attribution, participation_generation); they are **not** part of the freeze.

How the logical schema was established: `bd sql` is refused in embedded mode (`'bd sql' is not yet supported in embedded mode`, measured), and no Dolt CLI is installed, so `SHOW CREATE TABLE` on a live store is not possible here. The baseline is the July pure-Go SQLite translation `internal/storage/sqlite/schema.sql` at `0d7cf0022` (it matches upstream through main 0055 / ignored 0012 and passed the two-tier conformance harness then), plus every DDL statement in main 0056-0066 and ignored 0013-0026 at `c1c4b642a`, read one by one (list below). G1 (backend conformance) and W6 (CLI parity on five real corpora) are the proof that nothing was missed.

## Decision

### 1. Versioning

- `PRAGMA user_version = 1` is schema v1 = "upstream v1.3.1 logical schema + lite tables below". Created in one transaction on an empty file.
- Open rules (W2): `user_version = 0` and no tables: create. `= 1`: open. `> 1`: refuse with "this store was written by a newer beads-lite (schema N); this binary knows 1" and exit non-zero (spec edge case "newer data refused"). Any other mismatch: refuse, never guess.
- A future schema change is a numbered Go step `v1 -> v2` in `sqlitestore/schema.go`, never a replay of upstream migrations. Upstream migrations after 0066 are adopted only by a new ADR.
- `schema_migrations` is created and seeded with every v1.3.1 migration name (main 0001-0066, ignored 0001-0026) so upstream code that asks "are migrations pending?" (doctor, version checks) sees none. It is read-only after creation.

### 2. Type mapping (applies to every column)

| MySQL / Dolt | SQLite | Note |
|---|---|---|
| `varchar(n)`, `char(n)`, `text`, `longtext` | `TEXT` | Lengths are not enforced by SQLite; upstream validates lengths in Go (title 500). Keep the `n` in a comment. |
| `int`, `bigint`, `tinyint(1)` | `INTEGER` | Booleans stay 0/1. |
| `datetime` | `TEXT` `YYYY-MM-DD HH:MM:SS` UTC | Second precision, as Dolt v1.3.1 `datetime(0)` (0069's `DATETIME(6)` is post-1.3.1). Driver option `_time_format=datetime` (July `dsn.go`). Go truncates, Dolt rounds half-up: the parity diff normalises sub-second (known since July). |
| `json` | `TEXT` | Parity bar is semantic JSON equality, not byte order (Dolt may reorder keys). Callers parse JSON. |
| `DEFAULT CURRENT_TIMESTAMP` | same | SQLite writes `YYYY-MM-DD HH:MM:SS` UTC: matches. |
| `ON UPDATE CURRENT_TIMESTAMP` | dropped | issueops sets `updated_at` explicitly (July audit). |
| `INDEX ...` inside `CREATE TABLE` | separate `CREATE INDEX` | Same names. |
| `FOREIGN KEY ... ON DELETE CASCADE ON UPDATE CASCADE` | same | `PRAGMA foreign_keys=1` on every connection. |
| `CHECK (...)` | same | |
| collation `utf8mb4_0900_bin` (Dolt default, case-sensitive) | default `BINARY` | Plus `PRAGMA case_sensitive_like=1` for Dolt `LIKE` parity. |

### 3. Tables

Baseline = July `schema.sql` (tables `issues, leases, wisps, labels, wisp_labels, dependencies, wisp_dependencies, comments, wisp_comments, events, wisp_events, config, metadata, local_metadata, issue_counter, child_counters, wisp_child_counters, custom_statuses, custom_types, repo_mtimes`), with these v1.3.1 deltas applied:

| Source migration (v1.3.1) | Change to apply | Decision |
|---|---|---|
| main 0056 | `CREATE INDEX idx_comments_issue_created_id ON comments(issue_id, created_at, id)`; same `idx_wisp_comments_issue_created_id` on `wisp_comments` (also ignored 0014) | keep (keyset paging of comments; also our compact `show`) |
| main 0057 | `events.old_value`, `new_value` LONGTEXT | no-op in SQLite (TEXT) |
| main 0058 | `wisp_dependencies`: FKs `fk_wisp_dep_wisp_target (depends_on_wisp_id) -> wisps(id)`, `fk_wisp_dep_issue_target (depends_on_issue_id) -> issues(id)`, both CASCADE; `CHECK ck_wisp_dep_one_target ((depends_on_issue_id IS NOT NULL) + (depends_on_wisp_id IS NOT NULL) + (depends_on_external IS NOT NULL) = 1)` | keep; check that `dependencies` in July already carries the same split-target check, add it if not |
| main 0059, 0061; ignored 0015, 0018, 0026 | data repairs / markers only | nothing to apply (fresh store has no bad rows; migrated data comes from export, see 5) |
| main 0060, ignored 0020 | `storage_class TEXT` (varchar 16, NULL) on `issues` and `wisps` | keep |
| main 0062 | `events` moved under `dolt_ignore` (not versioned); id `CHAR(36)` UUID, no default (0051) | plain table; id TEXT NOT NULL PRIMARY KEY |
| main 0063 | new `provenance_events(id TEXT PK, issue_id TEXT NOT NULL, kind TEXT NOT NULL, actor TEXT, ref TEXT, ref_kind TEXT, payload TEXT, source TEXT NOT NULL, occurred_at TEXT, created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`, indexes `(issue_id, occurred_at)`, `(ref)`, `(kind)`, FK issue CASCADE | keep |
| main 0064, 0066; ignored 0022, 0023, 0025 | new `bd_events_journal(seq INTEGER PK, ts TEXT NOT NULL, op TEXT NOT NULL, issue_id TEXT NOT NULL, issue_json TEXT, dep_json TEXT, comment_json TEXT, actor TEXT NOT NULL DEFAULT '')`, indexes `(issue_id)`, `(ts)`; `bd_events_seq(id INTEGER PK, next_seq INTEGER NOT NULL)` seeded `(0, 0)` | keep (issueops writes it inside mutations; dropping it means editing upstream code). Bounded by upstream journal auto-prune, which W4 makes SQLite-safe. |
| main 0065; ignored 0021, 0024 | wider text columns on `wisps`/`wisp_comments` | no-op in SQLite |
| ignored 0013 | `wisps.row_lock INTEGER NOT NULL DEFAULT 0` | keep |
| ignored 0016 | `leases.granted_node TEXT NOT NULL DEFAULT ''` | keep |
| ignored 0017, 0023 | `CREATE INDEX idx_wisps_defer_until ON wisps(defer_until)` | keep |
| ignored 0019 | `events` created in the ignored set too (wisp-side events) | one `events` table, as July |

Wisp tables (`wisps`, `wisp_*`) are **kept, empty in practice**: Codesters does not use wisps (spec, out of scope), but issueops routes ephemeral writes to them; keeping the tables costs nothing and avoids editing that upstream code (W4 then only removes the Dolt-ignore assumptions).

Not created: every Dolt system table or view (`dolt_ignore`, `dolt_nonlocal_tables`, `dolt_log`, `dolt_diff_*`, `dolt_history_*`, `dolt_status`). Code that queries them is W4's (capability check or "unsupported for this backend").

### 4. Tables beads-lite adds (history, spec row 15 and FR-007)

Upstream answers `bd history <id> --json` from Dolt commits (`[]storage.HistoryEntry{CommitHash, Committer, CommitDate, Issue}`). rayeh's deploy gates read it (`tools/deploy/gate-lib.mjs:122-145`: oldest version of the current run where `Issue.assignee` is the owner and `Issue.status` is `owner_decision`/`in_review`). It is therefore **implemented, not stubbed**:

```sql
-- one row per (write transaction, issue touched in it)
CREATE TABLE lite_issue_versions (
  seq          INTEGER PRIMARY KEY,          -- CommitHash is rendered "lite-<seq>"
  issue_id     TEXT NOT NULL,
  committed_at TEXT NOT NULL,                -- CommitDate, UTC, second precision like Dolt
  committer    TEXT NOT NULL,                -- the bd actor of the writing process
  scalars      TEXT NOT NULL,                -- JSON object: every issues column except the 5 large ones, NULL/empty omitted
  text_hash    TEXT NOT NULL REFERENCES lite_version_text(hash)
);
CREATE INDEX idx_lite_issue_versions_issue ON lite_issue_versions(issue_id, seq);

-- content-addressed bundle of the 5 large columns, stored once however many versions share it
CREATE TABLE lite_version_text (
  hash TEXT PRIMARY KEY,                     -- sha256 of the canonical bundle
  body TEXT NOT NULL                         -- JSON {description, design, acceptance_criteria, notes, metadata}
);

-- scratch set filled by triggers, emptied by the store at commit
CREATE TABLE lite_touched (issue_id TEXT PRIMARY KEY);
CREATE TRIGGER lite_touch_ins AFTER INSERT ON issues BEGIN INSERT OR IGNORE INTO lite_touched VALUES (NEW.id); END;
CREATE TRIGGER lite_touch_upd AFTER UPDATE ON issues BEGIN INSERT OR IGNORE INTO lite_touched VALUES (NEW.id); END;
```

- Write path: the sqlitestore transaction wrapper, just before `COMMIT`, snapshots every row in `lite_touched` into `lite_issue_versions` (one version per command, like one Dolt commit per command) and empties `lite_touched`. Same transaction, so a version exists if and only if the write committed. No upstream file changes.
- Read path: `History(ctx, id)` returns versions newest first (Dolt order), `Issue` rebuilt from `scalars` + the text bundle. `--limit` as upstream. `bd history --events` already reads `events` and needs nothing new.
- Size (SC-008, < 100 MB after 30 days at 3,000 writes/day): 90,000 versions x ~250 B of scalars = ~23 MB; text bundles only when a large field changes. Estimate, to be measured by W7 on the 12x set; if over, add pruning (keep the first version of every status/assignee run) by a new ADR.
- Migration (W8) seeds `lite_issue_versions` from upstream `bd history <id> --json` so rayeh's "asked since" clocks survive the switch (section 5).

### 5. Data that the migration must carry and where it comes from

`bd export` (JSONL, upstream format) carries issues with comments, dependencies, labels and metadata. Not in it: `events` (use `bd export --events`, spec row 16), `config`/custom statuses/types (`bd config list --json` or the export header, W8 to check), Dolt history (`bd history <id> --json` per bead). `bd sql` cannot be used (refused in embedded mode). `local_metadata`, `repo_mtimes`, `leases`, `bd_events_journal`, `provenance_events`, wisp tables: W8 checks counts on each real corpus; if any is non-empty and not reachable through a supported command, W8 reports it before that project's switch (stop, not silent loss).

## Alternatives rejected

1. **Run the 66 + 26 upstream migrations through the dialect translator at open.** Keeps upstream parity for free on paper, but most of them are Dolt-specific (PREPARE/EXECUTE guards, `dolt_ignore`, `INFORMATION_SCHEMA` probes, Dolt CLI workarounds in `cli_migrations.go`); translating them is more code than the schema itself, and it re-creates the 6-minute first open.
2. **Adopt upstream main's `issue_versions` (0067-0071) for history.** It is post-1.3.1, part of a still-moving "versioned beads" design (attribution, participation, removed restrictions), and would tie our history format to it. Ours is three small tables in our own folder; revisit if we ever merge past 1.3.1.
3. **Full-row snapshot per version (no text dedupe).** Simplest, but at ~4 KB of large text per row it is ~360 MB after 30 days, breaking SC-008.

## Cost

Local disk only: $0 per active user, $0 per idle user, no quota, no process when idle. History adds ~23 MB per 90,000 writes (estimate, W7 measures).

## Exit path

`bd export` stays upstream JSONL, so stock bd 1.3.1 `bd import` takes the data back. History rows have no upstream equivalent and are lost on such an exit (upstream would start a fresh Dolt history); the archived Dolt store keeps the pre-switch history.
