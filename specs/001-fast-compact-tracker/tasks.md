---
description: "Task list for 001-fast-compact-tracker (beads-lite)"
---

# Tasks: Fast, compact, bd-compatible local tracker and board

**Input**: `specs/001-fast-compact-tracker/` (plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md) and [ADR 0001](../../docs/decisions/0001-frozen-sqlite-schema.md).

**Tests**: included. The spec gives an independent test per story and the constitution makes speed and compatibility release gates measured on real data.

**Format**: `- [ ] Txxx [P] [USn] (role; owns <folder>) description`. **[P]** = can run at the same time as other [P] tasks: it edits only the folder it owns and its blockers are done. "Owns" = only this task edits it; everyone may read. Two tasks never own the same file. Each engineer works in their own git worktree of `E:/Projects/beads-lite` (or `E:/Projects/beads-lite-ui`), branch `<role>/<task>`, rebased on `lite` before merge.

Repos: fork = `E:/Projects/beads-lite` (branch `lite`, based on v1.3.1 `c1c4b642a`); board = `E:/Projects/beads-lite-ui`; company = `~/company` (changes there go through company beads, not this list).

## Ready to start now (in parallel)

T004, T006, T007, T012, T014, T019, T022, T023, T032, T038. Already running: T001 (SRE), T002 (QA), T005 (SWE, W1), T034 (UX). T008 and T009 start the day T006 lands its `store.go` struct and commit hook.

---

## Phase 1: Setup

- [ ] T001 (sre; owns repo root, `scripts/lite-build.ps1`, CI) P0.1: Go 1.26.5 on the build PC, `lite` pinned at v1.3.1 (done: base `c1c4b642a`), `CGO_ENABLED=0 go build -o <fixed build dir>/bd.exe ./cmd/bd` works unchanged on Windows; record binary size and `bd version` time. On EPERM/antivirus refusal: stop and report. *In progress.*
- [ ] T002 [P] (qa; owns `parity/`) P0.2: golden corpus: copies of company, rayeh, fatura, abduu-tasks, whisper-flow stores + upstream bd 1.3.1 outputs as JSON for every row of `contracts/cli-compat.md` (list variants, show of each id, ready, blocked, comments, count, export, export --events, history --json for every id). *In progress.*
- [x] T003 [P] (cto; owns `docs/decisions/`) P0.3: frozen SQLite schema, `docs/decisions/0001-frozen-sqlite-schema.md`. *Done 2026-10-25.*
- [ ] T004 [P] (sre; owns `bench/`) timing and stress harness: whole-process median of 20 for show/list/ready/comments/count/create/update/comments add/close with the board open and 3 agents active; 12x generator (~19,400 beads, proportional comments/deps/labels); 10- and 20-writer stress; baselines on upstream today saved in `bench/results/baseline-2026-10.json`.

## Phase 2: Foundational storage core (blocks every user story)

- [ ] T005 [P] (swe A; owns `internal/storage/sqlitedialect/`) W1: revive the translator from `0d7cf0022`, extend to every MySQL-ism in today's issueops (ON DUPLICATE KEY, INSERT IGNORE, CONCAT, UTC_TIMESTAMP, JSON_ARRAYAGG, JSON_EXTRACT, JSON_UNQUOTE, DATE_FORMAT, FOR UPDATE, UPDATE aliases); table-driven test per construct. *In progress.*
- [ ] T006 [P] (swe B; owns `internal/storage/sqlitestore/{schema.sql,schema.go,dsn.go,open.go,store.go,transaction.go}`, `internal/configfile/`, `go.mod`/`go.sum`) W2: schema exactly per ADR 0001 (type mapping section 2, tables section 3, lite tables section 4, `PRAGMA user_version = 1`, refuse newer/unknown, seeded `schema_migrations`); DSN pragmas per research R3; refuse non-local-fixed disk; add `modernc.org/sqlite`; `backends.Register("sqlite", ...)`; remove the sqlite tombstone in `internal/configfile/backend_messages.go`/`configfile.go`. **Day-1 deliverable, pushed first**: the `Store` struct and a transaction wrapper with a `beforeCommit(tx)` hook list, so T008/T009 can build on it. Test: open/create/refuse cases in `internal/storage/sqlitestore/schema_test.go`.
- [ ] T007 [P] (swe D; owns only the 16 Dolt-SQL files in `internal/storage/issueops/`: as_of, diff, history, history_presence, federation, commit_pending, journal_autoprune, journal_prune, blocked_consistency, blocked_merge, wisp_routing, wake_defers, lease, claim, create, update) W4: make each safe on SQLite behind a capability check (no-op commit_pending, wisp routing to plain tables, journal prune without `dolt_*`); every change minimal and marked `// lite:`; Dolt behaviour unchanged (existing tests stay green).
- [ ] T008 (swe C; owns `internal/storage/sqlitestore/` except T006's and T009's files, incl. `conformance_test.go`, `unsupported.go`) W3: port each embeddeddolt role file (commenter, claimer, batch_*, counts, list_queries, deleter, metadata_cas, ready_claimer, releaser, slots, sweeper, iter, ...) as thin issueops calls; Dolt-only roles (version_control, federation, branch, diff) return the upstream "unsupported for this backend" error; wire `backend/conformance.RunAll` + `RunUnsupportedContract`. Blocked by T006 struct, T005.
- [ ] T009 [P] (swe E; owns `internal/storage/sqlitestore/{history.go,versions.go,history_test.go}`) W10: `lite_issue_versions` writer registered on T006's `beforeCommit` hook (snapshot every `lite_touched` row, text bundle deduplicated by sha256, empty `lite_touched`), `History(ctx,id)` and `--limit` per `contracts/history.md`; `IterEvents` from `events`. Tests: one version per command, none on rollback, newest first, text dedupe, `askedSinceFromHistory` sample from rayeh. Blocked by T006 struct.
- [ ] T010 (cto) Gate G1: conformance green on sqlitestore; review the XFAIL list (none may touch a row of `contracts/cli-compat.md`); record the verdict in `docs/decisions/0003-g1-xfail-review.md`. Blocked by T005, T007, T008, T009.

## Phase 3: US1 Agents read and write without waiting (P1)

**Goal**: SC-001, SC-004, SC-005. **Independent test**: quickstart 4 and 5.

- [ ] T011 [US1] (swe B; owns `cmd/bd/store_factory*.go`, `cmd/bd/init.go`, `internal/metrics/`) W5: `sqlite` default for `bd init`; open path for `sqlite` skips `acquireEmbeddedLock`; call `passthrough.MaybeExec()` (T012) before opening, and `switchstate.Check()` (T022) after resolving the project; metrics compiled off (default disabled, empty endpoint). Blocked by G1 (T010).
- [ ] T012 [P] [US1] (swe F; owns `internal/lite/passthrough/`) W12: when the resolved store is embedded Dolt, run upstream bd 1.3.1 (`BD_LITE_UPSTREAM` or fixed path) with identical args, env, stdio and exit code, `windowsHide`; tests with a stand-in exe, including the NoDefaultCurrentDirectoryInExePath case (company lesson). Research R6.
- [ ] T013 [US1] (sre; owns `bench/results/`) run T004 harness on fork vs upstream for all five corpora (migrated copies) and 12x; stress with backup snapshot running. Pass = SC-001, SC-004, SC-005 and the plan's internal targets. Blocked by T011, T020.

## Phase 4: US3 Existing scripts, hooks and prompts keep working (P1)

**Goal**: FR-001, FR-014, SC-009. **Independent test**: quickstart 2, 3, 11.

- [ ] T014 [P] [US3] (swe G; owns `cmd/bd/lite_mapped.go`, `cmd/bd/lite_backup.go`) W11: `dolt push/pull` and `doctor --fix` print the replacement and exit non-zero (row 22); `bd backup snapshot <file>` = `VACUUM INTO` (never copies the live file); other Dolt-only commands print "not supported in beads-lite" + pointer, non-zero (FR-014). Messages part can start now; snapshot needs T006.
- [ ] T015 [US3] (qa; owns `parity/runner/`, `test/conformance/profiles.go`) W6: parity runner over every row of `contracts/cli-compat.md` on each migrated corpus with the listed normalisation only; sqlite profile for the CLI differential runner. Blocked by T011, T020.
- [ ] T016 [US3] (qa; owns `parity/rayeh-history-replay.mjs`) rayeh gate replay: `askedSinceFromHistory` (imported from rayeh `tools/deploy/gate-lib.mjs`) on fork vs upstream `history --json` for every rayeh bead, before and after migration: all equal. Blocked by T009, T020.
- [ ] T017 [US3] (qa) SC-009: learner and link-inbox suites in `~/company` with `BD_BIN` = fork on a corpus copy, no other change; results in `parity/results/`. Blocked by T011.
- [ ] T018 [US3] (cto) Gate G2: T013 + T015 + T016 + T017 all pass; verdict appended to `docs/decisions/0003-g1-xfail-review.md`.

## Phase 5: US4 Move each project with nothing lost (P1)

**Goal**: FR-008, FR-009, SC-007. **Independent test**: quickstart 6.

- [ ] T019 [P] [US4] (swe H; owns `internal/migration/sqlitelite/extract*.go`) W8 extractor: on a copy, via upstream bd 1.3.1 subprocesses only (research R5): `export`, `export --events`, `config list --json`, `history <id> --json` per bead on N parallel copies, then delta of beads whose `updated_at` moved; counts of tables not covered (local_metadata, leases, provenance_events, bd_events_journal, wisps). Measure wall time per corpus into `bench/results/migration-extract.json`; if company history extraction > 30 min, raise the Dolt-CLI fallback to CTO.
- [ ] T020 [US4] (swe H; owns `internal/migration/sqlitelite/{load,verify}*.go`, `cmd/bd/lite_migrate.go`) W8 loader + verifier: write a new `.beads/beads.db` (never touch `embeddeddolt/`), seed `lite_issue_versions` from extracted history, per-bead verify report (counts of comments, deps with type, labels, metadata keys incl. `card_*`/`title_ar`/`asked_at`, events; byte-equal title/description/design/notes; custom statuses), then `show --json` diff for every id = 0; non-zero exit and live store untouched on any difference. Blocked by T006, T008, T009, T019.
- [ ] T021 [US4] (qa; owns `parity/roundtrip/`) quickstart 6 on all five copies: migrate, fork export, stock bd import on a scratch copy, re-export equal. Blocked by T020.

## Phase 6: US8 Switch every project safely, in order (P1)

**Goal**: FR-017, SC-012. **Independent test**: quickstart 7. Order (owner, 2026-10-25): rayeh, fatura, abduu-tasks, then company, whisper-flow and any other project found.

- [ ] T022 [P] [US8] (swe F; owns `internal/lite/switchstate/`, `cmd/bd/lite_switch.go`) W13: `.beads/lite-switch.json` states and `bd lite-switch announce|ack|status|freeze|migrate|flip|rollback` per `contracts/switch.md`; `switchstate.Check()` prints the notice / refuses writes with exit 3; freeze holds upstream's `.beads/embeddeddolt/.lock`; `migrate` calls T020. Package and tests can start now; `migrate` step after T020.
- [x] T023 [P] [US8] (sre; owns `docs/runbooks/switch-project.md`) runbook: active-session listing (Claude Code transcripts under the project folders and worktrees, last 2 h), CoS notice on a company bead, the one-time PATH/`BD_BIN` change, caller list (agents, git hook, Telegram bridge, learner, link-inbox, decision-inbox, backup), restart order, project docs to update (`dolt push`, `doctor --fix`), 24 h watch checks, rollback.
- [ ] T024 [US8] (qa; owns `parity/switch-dryrun/`) quickstart 7 dry run on copies with a scripted session writing before and during the freeze, through fork and through upstream bd; then a rollback. Blocked by T012, T020, T022.
- [ ] T025 [US8] (sre, via a company bead CoS opens) point PATH/`BD_BIN` at the fork once (passthrough keeps every not-yet-migrated project on upstream). Blocked by T018, T024.
- [ ] T026 [US8] (sre) switch **rayeh** per runbook; its deploy gates' history replay (T016) re-run on the live result. Blocked by T025.
- [ ] T027 [US8] (sre) switch **fatura**. Blocked by T026 + 24 h watch.
- [ ] T028 [US8] (sre) switch **abduu-tasks**. Blocked by T027 + 24 h watch.
- [ ] T029 [US8] (sre) switch **company** (`~/company`, largest store). Blocked by T028 + 24 h watch.
- [ ] T030 [US8] (sre) switch **whisper-flow**. Blocked by T029 + 24 h watch.
- [ ] T031 [US8] (sre) scan for any other project with `.beads/` and switch each the same way. Blocked by T030.

## Phase 7: US2 The board is instant and calm (P1)

**Goal**: FR-010, FR-011, SC-002, SC-003. **Independent test**: quickstart 8.

- [ ] T032 [P] [US2] (swe I; owns `E:/Projects/beads-lite-ui/server/`, branch off `main`, not `redesign`) W9: per `contracts/board-data.md`: one shared list call per refresh split by status (7 columns incl. owner_decision, waiting_for_test, waiting_for_merge, plus custom), shared in-flight promise, one refresh in flight, concurrency cap 4, watcher on `.beads/` (Dolt dir and `beads.db-wal`), bounded closed column, no post-mutation `show`, `windowsHide` on every spawn, loopback only. Works with upstream bd, so it can be deployed to the owner's board before any switch (log the deploy).
- [ ] T033 [US2] (qa; owns `bench/board-timing.mjs`) quickstart 8 on today's data and 12x, with upstream (today) and with the fork. Blocked by T032 (fork run also by T011).

## Phase 8: US7 A practical, modern board designed in Stitch (P1)

**Goal**: FR-016, SC-011. **Independent test**: quickstart 9.

- [ ] T034 [US7] (ux; owns `E:/Projects/beads-lite-ui/design/`, branch `redesign`) Stitch design covering every FR-010 column and action, wide and narrow windows, Arabic and English titles (RTL-safe), empty/loading/error states, owner_decision on the first screen. *In progress.*
- [ ] T035 [US7] (owner, card filed by CoS) approve the design; record the approval quoted and dated on the bead. Blocked by T034.
- [ ] T036 [US7] (swe J; owns `E:/Projects/beads-lite-ui/app/`) W15: client from the approved design on T032's data layer; link `#/issues?issue=<id>` opens the bead; each owner action (view, status, assignee, comment) in no more clicks than today. Blocked by T035, T032.
- [ ] T037 [US7] (ux + qa) UX sign-off built vs design; QA click count and SC-002/SC-003 re-run. Blocked by T036.

## Phase 9: US5 Compact, filterable output (P2)

**Goal**: FR-003, FR-004, SC-006 (text output; research R7). **Independent test**: quickstart 10.

- [ ] T038 [P] [US5] (swe K; owns `internal/lite/compact/`, and later the `// lite:` hooks in `cmd/bd/show.go`, `cmd/bd/list.go`) W14: text formatter with `show` <= 8 KB and `list` <= 50 lines plus "more" hints, `--full` = upstream text, `--fields a,b` for any `--json` command; `--json` default unchanged. Formatter package now; hooks after T011.
- [ ] T039 [US5] (qa) SC-006 on the 246 KB bead and company default list; JSON outputs unchanged in the parity run. Blocked by T038, T015.

## Phase 10: US6 Backups and history that do not slow work (P2)

**Goal**: FR-007, SC-004 with backup, SC-008. **Independent test**: quickstart 5 and 12.

- [ ] T040 [US6] (sre, through a company bead) `~/company/scripts/backup-projects.sh`: for migrated projects, `bd backup snapshot` + `bd export -o` instead of `bd dolt push`; runbook `knowledge/runbooks/backup-restore.md` updated. Blocked by T014; applied per project at its switch.
- [ ] T041 [US6] (sre; owns `bench/growth/`) SC-008: 30 days x 3,000 simulated writes on a company copy; store < 100 MB; if not, CTO decides pruning by ADR. Blocked by T009, T011.

## Final phase: Polish and cross-cutting

- [ ] T042 (sec + legal, once) clean-up pass after T031: loopback-only board, migration tool, freeze, no telemetry; LICENSE (MIT) + THIRD_PARTY_LICENSES with modernc (BSD-3). Findings are suggestions (company rule).
- [ ] T043 [P] (cto; owns `docs/decisions/`) ADR 0002 upstream-merge policy: pin v1.3.1, merge only by decision, `// lite:` marker rules, review of our diff size.

---

## Dependencies and execution order

- Phase 1 -> Phase 2 -> G1 (T010) -> US1 wiring (T011) -> US3 parity, US4 migration -> G2 (T018) -> US8 switches in owner order.
- Inside Phase 2: T006's day-1 struct unblocks T008 and T009; T005, T007 independent.
- US2 (T032) is independent of the fork and can ship first; US7 build (T036) waits for the owner's approval (T035).
- US5 formatter and US8 switch-state package start now; their CLI hooks land after T011.
- Each project switch waits for the previous project's 24 h watch.

## Parallel example (now)

```text
sre:     T004 bench harness          | T023 switch runbook
swe B:   T006 schema/open (day-1 struct first)
swe D:   T007 issueops Dolt-SQL files
swe F:   T012 passthrough, then T022 switch state
swe G:   T014 mapped commands
swe H:   T019 migration extractor + timing
swe I:   T032 board server data layer (beads-lite-ui)
swe K:   T038 compact formatter
running: T001 sre, T002 qa, T005 swe A, T034 ux
```

## Implementation strategy

MVP = Phase 2 + US1 + US3 + US4 on copies (G2 passed), then the rayeh switch. The board data layer (T032) ships to the owner's live board as soon as it is verified, since it helps today with upstream bd. The redesigned client follows the owner's design approval. Projects switch one at a time; each is reversible until its 24 h watch passes.
