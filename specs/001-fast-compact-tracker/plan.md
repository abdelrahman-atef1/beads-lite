# Implementation Plan: Fast, compact, bd-compatible local tracker and board

**Branch**: `lite` (spec dir `001-fast-compact-tracker`) | **Date**: 2026-10-25 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/001-fast-compact-tracker/spec.md` (owner decisions 2026-10-25), CTO draft `plan-draft.md` (upstream reading), `bd-surface.md`, `ui-calls.md`, ADR [0001 frozen SQLite schema](../../docs/decisions/0001-frozen-sqlite-schema.md).

## Summary

Fork bd at v1.3.1 and add one in-tree storage backend, `internal/storage/sqlitestore`, that runs the existing `issueops` logic on a single SQLite file through a MySQL-to-SQLite statement translator (`internal/storage/sqlitedialect`, revived from upstream's July 2026 pure-Go backend). `cmd/bd` stays upstream, so commands, flags, exit codes and JSON fields stay the same by construction; upstream's backend conformance kit and a CLI diff on our five real corpora prove it. The embedded Dolt engine, its process-wide lock and its per-write commit history go away; `bd history` is answered from a small version table of our own (ADR 0001 section 4), because rayeh's deploy gates read it. The board (fork of beads-ui 0.12.6, repo `beads-lite-ui`) makes one tracker call per refresh instead of seven and gets a new design made in Stitch by UX, built only after the owner approves it. Projects then switch one at a time, in the owner's order (rayeh, fatura, abduu-tasks, then company, whisper-flow and the rest), each with notice, freeze, migrate, verify, flip and a 24 h watch.

## Technical Context

**Language/Version**: Go 1.26.5 (`go.mod` of v1.3.1), built `CGO_ENABLED=0`. Board: Node.js (already installed, beads-ui's runtime).

**Primary Dependencies**: `modernc.org/sqlite` (pure Go, BSD-3; to be added to `go.mod` by W2: v1.3.1 only carries the CGO `mattn/go-sqlite3`, which we do not use); upstream `issueops`, `backend/conformance`, cobra CLI (unchanged).

**Storage**: one SQLite file per project, `.beads/beads.db`, WAL; schema frozen by ADR 0001 (`PRAGMA user_version = 1`).

**Testing**: `go test` (dialect table tests, `backend/conformance.RunAll` + `RunUnsupportedContract` on sqlitestore), CLI parity diff against the golden corpus in `parity/` (upstream bd 1.3.1 outputs on copies of the five real stores), timing and stress harness in `bench/`, existing learner and link-inbox suites (SC-009), board timing script.

**Target Platform**: Windows 11, the owner's PC, local NTFS disk; no console windows (`windowsHide`), no service.

**Project Type**: CLI + local storage library + loopback web board.

**Performance Goals**: spec SC-001..SC-005 (show/list/ready/comments/count <= 0.5 s today's data, <= 1.0 s on 12x; writes <= 0.5 s; board <= 2 s open, <= 2 s to reflect a change). Internal engineering targets, tighter, from the draft: `bd version` <= 80 ms, show <= 150 ms, list/ready <= 200 ms, write <= 120 ms, board full load <= 1 s; binary <= 60 MB.

**Constraints**: compat surface rows 1-22 unchanged (Principle V); zero lost writes with 10-20 parallel writers; WAL only on local fixed disks; never touch `embeddeddolt/` (archive, not delete); no network, no telemetry (upstream metrics compiled off).

**Scale/Scope**: today 1,619 / 1,034 / 536 / 304 / 90 beads (company, rayeh, fatura, abduu-tasks, whisper-flow), 6,426 comments in company; test at 12x (~19,400 beads); ~3,000 writes/day.

## Constitution Check

*GATE: checked before Phase 0 and again after Phase 1 design. Result: PASS with one justified deviation (Complexity Tracking).*

| Principle | How the plan complies | Status |
|---|---|---|
| I. Zero cost, zero idle cost | Local file, no service; board runs only while open; upstream metrics uploader compiled off (it is ON in today's install). $0 per active user, $0 per idle user, no quota. | PASS |
| II. Local-first | All data in `.beads/beads.db`; works offline; off-machine copy stays the hourly backup (export / `VACUUM INTO`), never a dependency. | PASS |
| III. Speed independent of task count | No per-write history commits, no process lock, single-file B-tree indexes; gates G2 and the board timing run on 12x data. | PASS (gated by measurement) |
| IV. Compact by default | Text output of `show`/`list` bounded by default with a "more" hint; `--json` keeps upstream shape (Principle V wins for machine output) plus an opt-in field selector. See research R7. | PASS |
| V. Compatible | `cmd/bd` is upstream; parity diff on every surface row; `bd history` implemented, not stubbed; `dolt push/pull`, `doctor --fix` mapped with a pointer (row 22). | PASS (gated by W6) |
| VI. No data loss, reversible | Migration reads through upstream bd, writes a new file, verifies counts and per-bead JSON; `embeddeddolt/` archived; `bd export` stays upstream JSONL; rollback per project. | PASS |
| VII. Boring tech, Windows first | SQLite + pure-Go driver, no native build, no console windows. Deviation: the Go toolchain is a build-time install on the build PC (see Complexity Tracking). | PASS with deviation |
| VIII. Safe by default | Board on 127.0.0.1 only; SQLite busy_timeout and immediate transactions, no lock that blocks readers; freeze refuses writes loudly. | PASS |

Post-design re-check (after data-model, contracts): unchanged. History table adds disk use within SC-008 by estimate; W7 measures it.

## Project Structure

### Documentation (this feature)

```text
specs/001-fast-compact-tracker/
├── spec.md            # PM (owner decisions)
├── plan-draft.md      # CTO draft, upstream reading (kept as source notes)
├── bd-surface.md      # commands our callers use
├── ui-calls.md        # beads-ui call map
├── plan.md            # this file
├── research.md        # Phase 0 decisions R1-R10
├── data-model.md      # entities -> tables (detail in ADR 0001)
├── quickstart.md      # how to prove each story end to end
├── contracts/
│   ├── cli-compat.md  # surface rows 1-22, exit codes, JSON fields
│   ├── history.md     # `bd history --json` contract (rayeh gates)
│   ├── switch.md      # announce / freeze / flip / rollback states
│   └── board-data.md  # board <-> tracker calls
└── tasks.md           # /speckit-tasks output
docs/decisions/0001-frozen-sqlite-schema.md
```

### Source Code (fork repo `E:/Projects/beads-lite`, branch `lite` at v1.3.1)

```text
internal/storage/sqlitedialect/      # W1: MySQL -> SQLite statement translator + driver wrapper
internal/storage/sqlitestore/        # W2 schema/open/store, W3 role wrappers, W10 history
├── schema.sql, schema.go            # W2 (ADR 0001)
├── dsn.go, open.go, store.go        # W2
├── history.go, versions.go          # W10 (lite_issue_versions)
└── <role>.go ...                    # W3 (ported from internal/storage/embeddeddolt)
internal/storage/issueops/           # W4: only the 16 Dolt-SQL files, marked // lite:
internal/configfile/                 # W2: sqlite backend accepted again
internal/lite/passthrough/           # W12: run upstream bd for stores not yet migrated
internal/lite/switchstate/           # W13: announce/freeze state read by every command
internal/migration/sqlitelite/       # W8: Dolt (via upstream bd) -> SQLite copier + verifier
cmd/bd/store_factory*.go, init.go    # W5: wiring, sqlite default, no embedded lock
internal/metrics/                    # W5: compiled off
cmd/bd/lite_*.go                     # W8 migrate-to-sqlite, W11 mapped commands, W13 switch, W14 compact output
parity/                              # QA: golden corpus + diff runner
bench/                               # SRE: timings, 12x generator, stress
test/conformance/profiles.go         # QA: sqlite profile for the CLI differential runner
```

Board (repo `E:/Projects/beads-lite-ui`, fork of beads-ui 0.12.6): `server/` (W9 data layer), `app/` (W15 client from the approved Stitch design in `design/`, branch `redesign`).

Company side (repo `~/company`, through company beads, not this repo): `scripts/backup-projects.sh` and callers' `BD_BIN`/PATH (SRE), agent docs that mention `dolt push`/`doctor --fix` (per project at its switch).

**Structure Decision**: our code lives in our own folders (`sqlitedialect`, `sqlitestore`, `internal/lite/*`, `internal/migration/sqlitelite`, `cmd/bd/lite_*.go`); upstream files are touched only for wiring (W5) and the 16 Dolt-SQL issueops files (W4), each change marked `// lite:` so later upstream merges stay small. Each work package owns its folder so several engineers can work at once (tasks.md marks them [P]).

## Phases (summary; tasks.md has the executable list)

0. **Repo and baselines** (SRE, QA, CTO): P0.1 toolchain + `lite` pinned at v1.3.1 + `CGO_ENABLED=0` build; P0.2 golden corpus + timing harness; P0.3 frozen schema (ADR 0001, done).
1. **Storage core**: W1 dialect, W2 schema/open, W3 role wrappers, W4 Dolt-only issueops paths, W10 history. Gate **G1**: conformance green on sqlitestore, XFAIL list reviewed by CTO, none touching a surface row.
2. **CLI**: W5 wiring + metrics off, W11 mapped commands, W12 passthrough, W14 compact text output; W6 CLI parity, W7 timings/stress. Gate **G2**: 0 parity diffs on all five corpora, all timing targets met.
3. **Migration and switch tooling**: W8 migrate-to-sqlite (+ history seed, verify report, export round trip), W13 switch states. Dry run of the whole switch on copies, including a session writing during the freeze.
4. **Board**: W9 server data layer (can ship first, works with upstream bd too), UX design approved by owner, W15 client build, UX sign-off.
5. **Switch, one project at a time**: rayeh, fatura, abduu-tasks, company, whisper-flow, then any other project found; each: notice, ack/stop, freeze, migrate, verify, flip, restart callers, 24 h watch.
6. **Clean-up pass**: SEC (loopback board, migration tool, no telemetry, freeze) and Legal (MIT LICENSE + THIRD_PARTY_LICENSES + modernc BSD-3) once, per company rule.

Critical path: P0.1 -> W2 -> W3/W4/W10 -> G1 -> W5 -> W6/W7 (G2) -> W8 + W13 dry run -> rayeh switch. Board (W9, W15) and UX run beside it. Estimate: 12-16 SWE runs, 4 SRE, 3 QA, 2 UX.

## Rollback (per project)

Until its 24 h watch passes: `metadata.json` backend back to `dolt`; the passthrough (W12) then sends that project's calls to upstream bd 1.3.1 and the untouched `embeddeddolt/` opens as before. Writes made on SQLite since the switch go back through fork `bd export` -> upstream `bd import`, then a diff. After the watch, the archived `.beads/` copy + JSONL stay 30 days; nothing is deleted without the owner.

## Risks

| Risk | Weight | Mitigation |
|---|---|---|
| Translator misses a MySQL-ism on a path tests did not run (July: `bd list --json` broke only end to end) | high | G1 + W6 on all five real corpora, every surface row |
| `is_blocked` propagation differs (upstream's most delicate semantic) | high | conformance transition fixtures; `ready`/`blocked` diff on every project |
| Migration cannot read some Dolt-only data: `bd sql` is refused in embedded mode (measured) | high | W8 reads through supported commands only (export, export --events, config, history); counts non-empty tables it cannot reach and stops before that switch |
| History seeding is slow (one `bd history` per bead, 0.6-10 s each) | medium | run on parallel copies before the freeze; at freeze, re-read only beads changed since (research R5) |
| We own a 359k-line fork; upstream moves fast | high | pin v1.3.1; our code in our folders + `// lite:` markers; merge upstream only by decision |
| Unsigned self-built exe scanned or refused by the antivirus | medium | build to a fixed path; on EPERM stop and report (company lesson), never retry |
| Freeze bypass: a caller still pointing at upstream bd writes to the old store | medium | switch tool holds upstream's exclusive embedded lock during the freeze, so upstream bd fails loudly (research R8) |
| WAL on a synced or network folder | low | open refuses a non-local-fixed disk |

## Complexity Tracking

| Violation | Why needed | Simpler alternative rejected because |
|---|---|---|
| Principle VII / SC-010 "no new installs": Go 1.26.5 toolchain on the build PC | bd is Go; the fork must be compiled. Build-time only: users and agents run one `bd.exe`, nothing to install at runtime. Free, no account. Install is SRE's P0.1 (in progress). | A prebuilt upstream binary cannot contain the SQLite backend; the Node rewrite (company-1bo5 option C) was rejected by the owner's "fork" decision and re-creates the JSON contract by hand. |
| Upstream-file edits in `issueops` (16 files) and `cmd/bd` wiring | Dolt SQL and the embedded lock live there | Rewriting issueops portable touches ~49 hot files and conflicts on every upstream merge; translation at the driver keeps the diff small. |
