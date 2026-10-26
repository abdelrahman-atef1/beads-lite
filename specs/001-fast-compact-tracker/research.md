# Research: decisions for 001-fast-compact-tracker

CTO, 2026-10-25. Upstream facts are from `plan-draft.md` section 1 (v1.3.1 = `c1c4b642a`). Measurements marked "measured" were taken on the owner's PC today.

## R1. Fork shape

- **Decision**: Go fork at v1.3.1; new backend `internal/storage/sqlitestore` with the embeddeddolt role-wrapper layout over the same `issueops`; MySQL-isms rewritten per statement by `internal/storage/sqlitedialect`; `cmd/bd` unchanged except wiring; `sqlite` default for `bd init`.
- **Rationale**: the storage seam (`storage.DoltStorage`, `backends.Register`, `backend/conformance`) already exists; the July pure-Go SQLite backend passed conformance and was removed for simplicity, not a defect. Commands and JSON stay upstream's by construction.
- **Alternatives**: revert the July commits (195-file drift, two stores to own); rewrite issueops in portable SQL (~49 hot upstream files, conflicts on every merge); own Node tracker (owner chose the fork); Dolt server mode (new always-on process, history growth stays).

## R2. SQLite driver

- **Decision**: `modernc.org/sqlite`, pure Go, added to `go.mod` by W2.
- **Rationale**: `CGO_ENABLED=0` (no C compiler on the PC; Principle VII); July's dialect wrapper was written for it.
- **Alternatives**: `mattn/go-sqlite3` (already in go.mod, needs CGO); `ncruces/go-sqlite3` (WASM, younger, different driver API from the July code).

## R3. Concurrency and locking

- **Decision**: one `.beads/beads.db` per project, WAL; pragmas `foreign_keys(1)`, `journal_mode(WAL)`, `busy_timeout(5000)`, `case_sensitive_like(1)`, `_txlock=immediate`, `_time_format=datetime`; one connection per process; upstream `acquireEmbeddedLock` skipped for this backend. Open refuses a store on a non-local-fixed disk.
- **Rationale**: readers never wait in WAL; writers queue inside SQLite for at most 5 s and a write is milliseconds, so the 20-writer stress (W7) is the proof. The process-wide lock is cause #1 of today's slowness (company-1bo5).
- **Alternatives**: keep a process lock (re-creates the queue); rollback-journal mode (readers block on writers).

## R4. History (`bd history`, spec row 15, FR-007)

- **Decision**: implemented, not stubbed: lite tables `lite_issue_versions` + `lite_version_text` + `lite_touched` (ADR 0001 section 4). One version per write transaction per touched issue, written in the same transaction; `History()` rebuilds `storage.HistoryEntry{CommitHash:"lite-<seq>", Committer, CommitDate, Issue}`. `--events` uses the existing `events` table.
- **Rationale**: rayeh deploy gates (`tools/deploy/gate-lib.mjs:122-145`) compute "how long the owner has been asked" from `[ {CommitDate, Issue:{assignee,status}} ]`; a stub would silently return `[]` there (the gate catches errors and treats them as "no history"), which is a wrong answer, not a loud one. Owner's Q1 default (per-bead history plus hourly backups, no whole-tracker time travel) is exactly this.
- **Alternatives**: rebuild versions from `events` (events do not carry a full row; fields not in an event would be wrong); upstream main's `issue_versions` (post-1.3.1, still moving); full-row snapshots (~360 MB/30 days, breaks SC-008).

## R5. Migration source (W8)

- **Superseded in part (2026-10-26)** by [ADR 0003](../../docs/decisions/0003-migration-extract-via-dolt-cli.md): history, events and tables bd cannot read now come from one pinned Dolt CLI batch on the copy; `bd export` and `bd config list` stay. The text below is the original plan.

- **Decision**: read the Dolt store only through upstream bd 1.3.1 as a subprocess, on a copy: `bd export` (issues, comments, deps, labels, metadata), `bd export --events`, `bd config list --json` (prefix, custom statuses/types), `bd history <id> --json` per bead (seeds `lite_issue_versions`). Write a new `.beads/beads.db`; never touch `embeddeddolt/`. History is read before the freeze on N parallel copies; at the freeze, only beads whose `updated_at` moved since are re-read.
- **Rationale**: `bd sql` is refused in embedded mode (measured: `'bd sql' is not yet supported in embedded mode`), no Dolt CLI is installed, and the fork has no CGO, so it cannot open embedded Dolt itself. Supported commands are the only reader we have. The freeze window must stay short: a fresh upstream `bd init` took 5 min 57 s and one `bd sql` call 62 s to fail (measured), so anything per bead runs before the freeze.
- **Alternatives**: July's table copier `2c0eb87bc` (needs a CGO build: no C compiler); install the Dolt CLI for one query on `dolt_diff_issues` (faster, but a new install; keep as fallback if W8's measurement shows per-bead history too slow: owner/SRE decision).
- **Open item for W8**: count rows in `local_metadata`, `leases`, `provenance_events`, `bd_events_journal`, wisp tables on each corpus (via whatever supported command exposes them); a non-empty table with no supported reader stops that project's switch with a report.

## R6. Projects not yet migrated (passthrough)

- **Decision**: the fork's `bd.exe` is what PATH/`BD_BIN` point to from the first switch on. When the store it finds is embedded Dolt (`metadata.json` backend `dolt`, no `beads.db`), it runs upstream bd 1.3.1 with the same arguments, environment, stdin/stdout/stderr and exit code (`internal/lite/passthrough`, upstream path from `BD_LITE_UPSTREAM`, else a fixed install path). Started with `windowsHide`.
- **Rationale**: a `CGO_ENABLED=0` fork cannot open embedded Dolt; migrating one project at a time means both stores live side by side for days. One PATH change (SRE, once) instead of per-project repointing of ~30 callers; rollback of a project is one `metadata.json` edit.
- **Alternatives**: per-project `BD_BIN` (every caller and hook must know which project is where; error-prone); keep both binaries on PATH under different names (callers hard-code `bd`).

## R7. Compact by default vs compatible output

- **Decision**: bounds apply to **text** output: `show` <= 8 KB (long fields and comment list cut with "N more comments, use --full"), `list` <= 50 lines with "M more, use --limit 0". `--json` output keeps upstream's shape and size unless the caller adds the new opt-in `--fields a,b,c`. `--full` restores the upstream text output.
- **Rationale**: Principle V outranks IV for machine callers: learner, link-inbox, hooks and the board read `--json` and need every field they read today. Agents mostly read text, which is where the 246 KB dumps hurt.
- **For PM**: SC-006 is then measured on text output; `--json` size is cut by `--fields`. If PM wants JSON bounded by default too, that is a spec change that breaks Principle V and needs the owner.

## R8. Notice and freeze during a switch (US8)

- **Decision**: per project, a state file `.beads/lite-switch.json` that every fork command reads at start (cheap stat):
  - `announced` (notice, with planned freeze time): every command in that project prints one stderr line "beads-lite: <project> switches at HH:MM; finish, then run `bd lite-switch ack`"; writes still work. `ack` records actor and time.
  - `frozen`: writes are refused with exit 3 and a clear message; reads work. The switch tool also holds upstream's exclusive `.beads/embeddeddolt/.lock`, so any caller still pointing at upstream bd fails loudly instead of writing to the old store.
  - `done` / `rolled-back`: file removed after the flip or rollback.
  The switch starts when every listed session has acked, or its last write is older than the announced window (default 15 min). Who is "active": sessions with a Claude Code transcript under that project's folders (including worktrees) changed in the last 2 h, listed in the switch report; CoS posts the notice in the company channel (beads) as well.
- **Rationale**: there is no way to message another chat directly; every session does reach the tracker, so the tracker carries the notice. The lock trick makes the old store refuse writes without changing upstream bd.
- **Alternatives**: rename `embeddeddolt/` during the freeze (upstream bd might then create a fresh empty store silently: unsafe); read-only file attributes (partial, Dolt may still write its journal).

## R9. Board

- **Decision**: fork beads-ui 0.12.6 (repo `beads-lite-ui`). Server first (W9): one `bd list --json --tree=false --limit 0` per refresh split by status, plus `ready`/`blocked`; one shared in-flight promise; skip a refresh while one runs; concurrency cap 4 instead of the global one-at-a-time queue; watch `.beads/` for both `embeddeddolt/` changes and `beads.db-wal`; closed column bounded (FR-011). Works against upstream bd too, so it can ship before any project switches. Client (W15) rebuilt from the Stitch design on branch `redesign` only after the owner approves it (FR-016); URL scheme `127.0.0.1:3000/#/issues?issue=<id>` kept.
- **Rationale**: `ui-calls.md`: 7 serial bd spawns per change is the 30-90 s refresh.
- **Alternatives**: read `beads.db` directly from Node (a second implementation of the data contract; breaks on schema change); a `bd serve` HTTP API (refused in embedded mode upstream; an always-on process).

## R10. Backup and the `dolt push` step

- **Decision**: the hourly backup stops calling `bd dolt push`; it runs `bd export -o` (already there) plus `VACUUM INTO <file>` through a new `bd backup snapshot` (W11), never copying the live `.db`/`-wal`. `bd dolt push/pull` and `bd doctor --fix` print the replacement command and exit non-zero (row 22).
- **Rationale**: WAL readers do not block, so the backup no longer holds a lock for 7-109 s; a raw file copy of a WAL database can be torn.

## R11. Telemetry

- **Decision**: upstream metrics compiled off in the fork (default disabled, empty endpoint). Today's install has them on (`metrics.disabled: false`, uploader in `internal/metrics/spawn.go`); `BD_DISABLE_METRICS=1` for the live install is SRE's, outside this repo.
- **Rationale**: Principles I, II, VIII.
