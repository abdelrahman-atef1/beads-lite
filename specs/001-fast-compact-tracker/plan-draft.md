# beads-lite: technical plan draft (CTO, 2026-10-25)

Owner decision (2026-10-25, chat, via CoS): fork beads + beads-ui, replace Dolt with SQLite from the start, keep the bd commands and JSON output, move all data. Not tracked in beads; Spec Kit once SRE's repo exists (E:/Projects/beads-lite, not touched by this draft).
Inputs: measurements in company-1bo5 design (read only); upstream read at `scratchpad/beads-lite/cto-upstream` (blobless clone, checkout v1.3.1 = c1c4b642a, the version installed here); `bd-surface.md` and `ui-calls.md` in the same folder.

## 1. What upstream looks like (v1.3.1)

Size: ~359k lines of non-test Go; storage ~90k; cmd/bd ~138k.

**The seam already exists.**
- `internal/storage/storage.go`: `Storage` (core, ~102 methods), `Transaction` (~34), and `DoltStorage` (line 648) = `Storage` + 14 sub-interfaces (VersionControl, HistoryViewer, RemoteStore, SyncStore, FederationStore, BulkIssue, DependencyQuery, EventQuery, Annotation, ConfigMetadata, Compaction, AdvancedQuery, FastStatistics, IssueLifecycle). Every command receives a `storage.DoltStorage`.
- `internal/storage/backends/backends.go`: name-keyed registry, `Backend{Open, OpenReadOnly, WorkspaceIsBeadsDir}`; public alias in `backend/backend.go` ("out-of-tree storage backends", `backend.Register` at init).
- `cmd/bd/store_factory.go:58` `newRegisteredBackendStore` opens any registered backend; `metadata.json` `backend` picks it.
- `backend/conformance/` (53 files, ~43k lines): `RunAll(t, factory)` + `RunUnsupportedContract`, the oracle that a backend behaves like the Dolt reference. `test/conformance/` has a CLI differential runner (idle since SQLite was removed).

**Where Dolt is wired in.**
- `internal/storage/embeddeddolt/` (~6.5k lines): the store we use today. Most files are thin role wrappers over `issueops` (commenter, claimer, batch_*, counts, list_queries...). Dolt-specific code is concentrated in `store.go` (open, commit per write), `version_control.go`, `federation.go`, `open.go`.
- `internal/storage/dolt/` (~17k lines): server-mode client (MySQL wire).
- `internal/storage/issueops/` (~24k lines, 89 files): the real logic, written against `DBTX` (`database/sql` Exec/Query/QueryRow), MySQL dialect. Dolt SQL only in 16 files: as_of, diff, history, history_presence, federation, commit_pending, journal_autoprune/prune, blocked_consistency/merge, wisp_routing, wake_defers, lease, claim, create, update (the last four are comments/ignore-table notes). MySQL-isms: ON DUPLICATE KEY x15, INSERT IGNORE x15, CONCAT x11, UTC_TIMESTAMP x9, JSON_ARRAYAGG x6, JSON_EXTRACT x4, JSON_UNQUOTE x3, DATE_FORMAT x1, FOR UPDATE x1.
- `internal/storage/schema/`: 128 MySQL-dialect migrations + Dolt ignore-table seeding (wisps live in dolt_ignore'd tables).
- 26 cmd/bd files type-assert to Dolt capabilities (VC, remote, history); 10 call DOLT_COMMIT/ADD/status directly (dolt, vc, sync, backup, doctor families).
- Lock: `cmd/bd/store_factory.go:127` `acquireEmbeddedLock` = exclusive TryLock on `.beads/embeddeddolt/.lock`; "the embedded backend supports only one writer at a time". This is cause #1 in company-1bo5.
- `bd serve` (HTTP API, `internal/httpapi`) refuses embedded mode permanently (`cmd/bd/serve.go:554`).

**An SQLite backend existed twice; it can be revived in part.**
1. "Beads Classic" SQLite (the original store, CGO mattn) removed 2026-02-15 (`a3bf3ee38`, Phase 6 of the Dolt move). Too old to revive; only its JSONL era is useful history.
2. Pure-Go SQLite behind the seam, `25d58b679` (2026-07-06) to removal `6fd9dbda7` + `2f5cfb3e0` (2026-07-18). Never in a tagged release. It passed the two-tier conformance harness against Dolt "with only the audited deferrals as XFAIL". Removed only to "consolidate on a single engine and dialect and keep Beads simple", not for a defect. Last tree with it: `0d7cf0022`. Files worth taking:
   - `internal/storage/sqlitedialect/translate.go` (15 KB) + `driver.go`: a modernc driver wrapper that rewrites the MySQL-isms per statement (INSERT IGNORE, ON DUPLICATE KEY -> ON CONFLICT, CONCAT -> ||, NOW/UTC_TIMESTAMP, JSON_ARRAYAGG -> json_group_array, JSON_UNQUOTE, DATE_FORMAT -> strftime, UPDATE alias rules).
   - `internal/storage/sqlite/schema.sql` (14 KB, MySQL schema translated), `dsn.go` (pragmas, see 3), `fromconfig.go`, `unsupported.go` (the list of methods it really implements; the rest return `storage.ErrUnsupported`).
   - `2c0eb87bc` "Add embedded Dolt to SQLite backend migration": `issueops/backend_migration.go` (616 lines) + `sqlite/backend_migration.go` (496) + test: a table-level copier from embedded Dolt to SQLite.
   - Not worth taking: `internal/storage/sqlkit` (a second generic SQL store). Drift since removal is 138 commits / 195 files / +36.7k lines in issueops+storage.go+schema, so a plain revert will not apply; we rebuild the store on today's embeddeddolt shape instead (see 4).

## 2. Decision for the fork's shape

**Chosen: Go fork pinned at v1.3.1, new in-tree backend `internal/storage/sqlitestore` that copies the embeddeddolt role-wrapper layout over the same `issueops`, with the revived `sqlitedialect` translator and a frozen SQLite schema; built `CGO_ENABLED=0`; `sqlite` is the default backend for `bd init`.** `cmd/bd` is untouched except wiring, so the commands, flags and JSON output stay the same by construction, and the upstream conformance kit proves it.

Rejected:
- **Revert the July commits onto v1.3.1 (revive sqlkit).** 195-file drift means a conflict-heavy merge of a parallel store that duplicated logic now living in issueops roles; we would own two stores' worth of code.
- **Rewrite issueops SQL as portable SQL instead of translating.** Touches ~49 files of the hottest upstream code, so every later upstream merge conflicts. Translation at the driver keeps the diff in our own folders.
- **Own Node tracker (company-1bo5 option C) / Dolt server mode.** C re-creates the JSON contract by hand; the owner chose the fork. Dolt server needs a new install plus an always-on process and keeps the history growth.

Cost (company ceiling): internal tool, local disk only: $0 per active user, $0 per idle user. No quota. One new install: Go toolchain 1.26.5 (free; `go.mod` line 3) on the build machine; no C compiler needed (modernc is pure Go). Installing it is the owner's/SRE's call.
Exit path: `bd export` JSONL stays the upstream format, so upstream bd (Dolt) `bd import` takes us back; the fork keeps the Dolt code compiled in server mode, so `metadata.json backend: dolt` still works.

## 3. Concurrency and locking (SQLite)

- One file `.beads/beads.db` per project, WAL. Pragmas from the July `dsn.go`: `foreign_keys(1)`, `journal_mode(WAL)`, `busy_timeout(5000)`, `case_sensitive_like(1)` (Dolt LIKE parity), `_txlock=immediate` (writers lock up front, no upgrade deadlock), `_time_format=datetime` (Dolt datetime(0) parity). Single connection per process.
- Readers never wait; writers queue in SQLite for at most 5 s (a write is ~4 ms, so contention is theoretical at our scale). No process-level exclusive lock: `acquireEmbeddedLock` is skipped for this backend.
- beads-ui's global one-bd-at-a-time queue (`bd.js:15-19`) becomes unnecessary; keep a small concurrency cap (4) instead of removing the queue.
- Windows: WAL needs a local NTFS disk (never a network share or a cloud-synced folder); `-wal`/`-shm` files sit beside the DB and must be gitignored and never copied alone.
- Backups: never copy the live `.db` file; use `VACUUM INTO <file>` or the JSONL export (backup-push already uses export).
- History: Dolt's per-write commit (11,750 commits in 4 days) is gone. Audit stays in the `events` table, which issueops already writes inside the mutation transaction (`storage.go` GetAllEventsSince; PROPOSAL 2.3). `bd history`/`diff`/`vc`/`dolt *`/`federation`/`branch` return the upstream "unsupported for this backend" stub.

## 4. Startup cost (why bd is slow before it touches data) and targets

Measured today (company-1bo5): bd.exe start 0.5-1.0 s (157 MB binary, embedded Dolt + go-mysql-server linked), Dolt open grows with history, plus the single-process lock.
Removed by the fork: the embedded Dolt engine from the binary (`CGO_ENABLED=0` drops the cgo-only embedded path; see `cmd/bd/store_factory_nocgo.go`), the Dolt open, the lock, and the metrics flusher.
Also found: upstream metrics are ON here (`~/.config/bd/config.yaml` `metrics.disabled: false`, endpoint gastownhall-eventsapi.com; `internal/metrics/spawn.go` spawns an uploader; queued batches in `~/.beads/eventsData`). The fork compiles metrics off (default disabled, endpoint empty). The live install needs `BD_DISABLE_METRICS=1` now (bead for sre filed, see report).

Success timings (company data, this PC, whole process, median of 10, with an agent load of 5 parallel bd readers running):

| command | today live | target |
|---|---|---|
| bd version | 600-1000 ms | <= 80 ms |
| show <id> (compact) | 8.3-9.9 s | <= 150 ms |
| list (default) | 7.0-9.7 s | <= 200 ms |
| ready | 10.9-23.4 s | <= 200 ms |
| write (update/comment/close) | 1.2-1.9 s + lock wait | <= 120 ms |
| board full load (7 columns + detail) | 30-90 s | <= 1 s |
| 12x synthetic data (19,428 beads): list / ready / show | - | <= 400 / 250 / 150 ms |
| 20 parallel writers x 50 writes | - | 0 "database is locked", 0 lost writes |

Binary size goal: <= 60 MB (measure; Defender scans every new exe on launch).

## 5. Phases and work packages

Each package owns distinct folders so several SWEs can work at once. "Owns" = only this package edits it; everyone may read.

**Phase 0 - repo and baselines (SRE + CTO; blocks all).**
- P0.1 (sre): fork repo (private), import upstream at v1.3.1 with history, branch `lite`, Go 1.26.5 on the build PC, `CGO_ENABLED=0 go build ./cmd/bd` works unchanged on Windows. Owns: repo root, CI/build scripts.
- P0.2 (sre/qa): golden corpus. For each project (whisper-flow, abduu-tasks, fatura, rayeh, company): a copied store, upstream bd 1.3.1 outputs saved as JSON for every command in `bd-surface.md` (list variants, show each id, ready, blocked, comments, count, export) = the parity oracle. Plus the timing harness (section 4). Owns: `parity/`.
- P0.3 (cto): frozen SQLite schema spec: dump `SHOW CREATE TABLE` for every table of a fresh v1.3.1 `bd init` (incl. ignored wisp tables, events, config, metadata, slots), diff against July `schema.sql`, decide each column. Owns: `docs/decisions/`.

**Phase 1 - storage core (parallel after P0).**
- W1 (swe A) dialect: revive `internal/storage/sqlitedialect` from `0d7cf0022`, extend to every MySQL-ism listed in section 1 against today's issueops; table-driven tests per construct. Owns: `internal/storage/sqlitedialect/`.
- W2 (swe B) schema + open: `internal/storage/sqlitestore/{schema.sql,schema.go,dsn.go,open.go,store.go}`, `PRAGMA user_version` instead of the 128 migrations, `backends.Register("sqlite", ...)` in an init, remove the sqlite tombstone in `internal/configfile/backend_messages.go`/`configfile.go`, skip the embedded lock. Owns: `sqlitestore/{schema*,dsn,open,store}.go`, `internal/configfile/`.
- W3 (swe C) role wrappers: port each embeddeddolt role file (commenter, claimer, batch_*, counts, list_queries, deleter, metadata_cas, ready_claimer, releaser, slots, sweeper, transaction, iter...) to `sqlitestore/` as thin issueops calls; Dolt-only roles (version_control, federation, history) go to a generated unsupported shell. Owns: `sqlitestore/` except W2's files. Depends on W2's `store.go` struct (agree the struct on day 1).
- W4 (swe D) Dolt-only paths in issueops: make the 16 Dolt-SQL files safe on SQLite (capability checks, no-op commit_pending, wisp routing to plain tables, journal prune without dolt_*). Owns: those 16 issueops files only. Keep each change minimal and marked `// lite:` for upstream merges.
- Gate G1: `backend/conformance.RunAll` + `RunUnsupportedContract` green on sqlitestore (XFAIL list reviewed by CTO, none touching a command in bd-surface.md).

**Phase 2 - CLI and build (after G1).**
- W5 (swe B) wiring: `bd init` default `sqlite`; `cmd/bd/store_factory*.go` arms; `doctor`, `backup`, `dolt`, `vc` commands degrade with the upstream stub message; metrics compiled off (`internal/metrics` default disabled, empty endpoint). Owns: `cmd/bd/store_factory*.go`, `cmd/bd/init.go`, `internal/metrics/`.
- W6 (qa) CLI parity: run the P0.2 corpus against the fork on migrated copies; JSON diff with volatile normalisation (timestamps, ids where minted) = 0 differences for every command our scripts use; plus `test/conformance` CLI differential runner re-enabled with a sqlite profile. Owns: `parity/`, `test/conformance/profiles.go`.
- W7 (sre) timings + concurrency: section 4 table on all five projects and the 12x set; 20-writer stress. Owns: `bench/`.
- Gate G2: W6 = 0 diffs, W7 all targets met.

**Phase 3 - migration tool (parallel with Phase 2, after W2).**
- W8 (swe E): `bd migrate-to-sqlite` in the fork. Source read without CGO, in this order of preference: (a) upstream bd 1.3.1 as a subprocess: `bd export` (issues with comments, deps, labels, metadata; memories) + `bd config list --json` (custom statuses, prefix, types) + `bd sql --json` dumps for `events`, `local_metadata`, slots and wisp tables; (b) the July table copier (`2c0eb87bc`) if a CGO build is available. Writes a new `.beads/beads.db`, never touches `embeddeddolt/`. Owns: `cmd/bd/migrate_sqlite*.go`, `internal/migration/sqlitelite/`.
- Verify (built into the tool): per-bead counts of comments, deps, labels, metadata keys, events; byte-equal titles/descriptions/design/notes; custom statuses present; then upstream-vs-fork JSON diff of `show` for every id = 0.

**Phase 4 - beads-ui fork (parallel from Phase 1; only needs a working fork binary to test).**
- W9 (swe F): fork beads-ui 0.12.6 (folding in `scripts/bdui-patch-board.mjs`: 7 columns incl. owner_decision / waiting_for_test / waiting_for_merge), keep the URL scheme `127.0.0.1:3000/#/issues?issue=<id>`, replace 7 spawns per refresh with one `bd list --json --tree=false --limit 0` split by status (+ ready/blocked), shared in-flight promise, queue concurrency 4, watcher on `beads.db-wal` instead of the Dolt dir. Owns: the beads-ui fork repo/folder. Success: board full load <= 1 s, refresh <= 300 ms on company data.

**Phase 5 - switch, one project at a time (sre; owner-visible).**
Order: whisper-flow -> abduu-tasks -> fatura -> rayeh -> company. Per project, in a quiet window: stop the board and the backup task for that project; `bd export` snapshot + copy of `.beads/` to an archive folder (kept, never deleted); migrate; verify (W8 checks); flip `metadata.json backend: sqlite`; repoint `BD_BIN`/PATH for agents, git hook, Telegram bridge, learner, link-inbox, backup scripts to the fork binary; run the scripts' own tests (learner, link-inbox, decision-inbox, backup); restart board; 24 h watch, then next project.

**Phase 6 - clean-up pass.** SEC (board server loopback-only, migration tool, no telemetry) and Legal (beads is MIT; keep LICENSE + THIRD_PARTY_LICENSES, add modernc BSD-3) once, at the end, per company rule.

Critical path: P0 -> W2 -> W3/W4 -> G1 -> W6 -> switch. W1, W8, W9 run beside it. Estimate: 10-14 SWE runs + 3 SRE + 2 QA.

## 6. Rollback

- Per project, until the 24 h watch passes: set `metadata.json backend` back to `dolt` and point `BD_BIN` at upstream 1.3.1; the untouched `embeddeddolt/` store opens as before. Writes made on SQLite since the switch are carried back with fork `bd export` -> upstream `bd import` (the JSONL is upstream format), then diffed.
- After the watch: the archived `.beads/` copy + JSONL stay for 30 days (no permanent deletion without the owner).
- The fork never edits or deletes `embeddeddolt/`; the Dolt GitHub remote push simply stops.

## 7. Risks

| risk | weight | mitigation |
|---|---|---|
| Translator misses a MySQL-ism on a path the tests did not run (July: `bd list --json` broke only end-to-end) | high | G1 + W6 CLI diffs on all five real corpora, every command in bd-surface.md |
| Denormalised `is_blocked` propagation differs (PROPOSAL 2.3: most dangerous semantic) | high | conformance transition fixtures; `ready`/`blocked` diff on every project |
| We own a 359k-line Go fork; upstream moves fast (138 storage commits in 10 weeks) | high | pin v1.3.1, merge upstream only on purpose, keep our diff in our own folders + `// lite:` markers; budget one merge run per quarter, or freeze |
| Data loss in migration (events, config, local metadata not in `bd export`) | high | W8 dumps them separately and verifies counts; Dolt store archived, never deleted |
| Go toolchain is a new install; unsigned self-built exe may be scanned/blocked by the antivirus (lesson: EPERM = stop) | medium | owner/SRE approve install; build to a fixed path; if Defender refuses, stop and report |
| Timestamp rounding differs (Go truncates, Dolt rounds half-up) | low | known from July (`testAuditImportCommentSubSecond`); normalise in diff |
| WAL on a synced/network folder corrupts | low | doctor check: refuse if the `.beads` path is not a local fixed disk |
| `bd serve` stays refused (it hard-codes the embedded refusal) | low | not needed; revisit only if the board wants an API |
