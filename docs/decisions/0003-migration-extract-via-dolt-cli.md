# ADR 0003: Migration extraction reads the Dolt store with a pinned Dolt CLI

- Status: accepted (CTO, 2026-10-26). Plan tasks T019 and T020 of `specs/001-fast-compact-tracker`. Supersedes the history/events/uncovered-table part of research R5 and of ADR 0001 section 5.
- Governs: `internal/migration/sqlitelite/extract*.go` (T019) and what `load*.go`/`verify*.go` (T020) receive.

## Context

T019 built the extractor R5 asked for: upstream bd 1.3.1 subprocesses only, on copies. Measured on a whisper-flow copy (`bench/results/migration-extract.json`, branch `swe/t019`): 96 beads, 292 bd calls, **1,147 s total, 1,091 s of it per-bead `bd history`** (4 workers). Two gaps:

1. **History is too slow to be usable on company.** bd 1.3.1 has no `export --events`, so events need `bd history <id> --events --json` per bead as well. `bd history` takes exactly one id (`cmd/bd/history.go`, `cobra.ExactArgs(1)`), and its query (`internal/storage/issueops/history.go`) is `SELECT ... FROM (SELECT * FROM dolt_history_issues) h WHERE h.id = ?`: every call scans the whole `dolt_history_issues` view, which holds one row per (commit, existing issue). One call measured 72.9 s on an idle-ish copy (it also triggered an auto-export: a read with a write side effect).
2. **Two non-empty tables have no reader.** No bd 1.3.1 command reads `local_metadata` or `leases`, and both are non-empty on real corpora (counts below). Under R5 that stops the switch of every project.

`bd sql` is refused in embedded mode; the fork is `CGO_ENABLED=0` and the embedded Dolt code is `//go:build cgo` (158 files in `internal/storage/embeddeddolt`); there is no C compiler on the machine.

### Measurements (2026-10-26, copies in the CTO scratch folder, busy PC)

The embedded store is a plain Dolt repository: `.beads/embeddeddolt/<db>/.dolt` (noms, repo_state.json). The Dolt CLI opens it as is.

| | whisper-flow | company |
|---|---|---|
| issues | 123 | 1,686 |
| Dolt commits (`dolt_log`) | 894 | 12,132 |
| merge commits | 0 | 0 |
| `dolt_history_issues` rows (what upstream `bd history` walks) | 46,105 | not run; estimate ~10 M (12,132 commits x 1,686 issues / 2) |
| distinct issue states in those rows | 285 | - |
| `dolt_diff_issues` rows (added + modified + removed) | 329 (123 + 206 + 0) | 5,382 (1,686 + 3,696 + 0) |
| `events` | 346 | 1,377 |
| `local_metadata` / `leases` | 222 / 15 | 767 / 39 |
| `wisps`, `bd_events_journal`, `provenance_events`, `repo_mtimes`, `interactions`, `issue_snapshots`, `compaction_snapshots`, `routes`, `federation_peers`, `wisp_events` | 0 | 0 |
| uncommitted (`dolt_status`, `to_commit='WORKING'`) | - | 0 |
| **one `dolt sql` batch: issue versions from `dolt_diff_issues` + `events` + `local_metadata` + `leases`** | - | **72.9 s, 44.8 MB JSON** |
| full `dolt_history_issues` dump (projected columns) | 59.8 s, 112 MB | - |
| store copy (`cp -r`) | - | 59.6 s (928 MB) |

Check of shape: for `whisper-flow-l8t.5.5`, upstream `bd history --json` returned 2 entries; the Dolt view returned the same 2 commit hashes and dates (Dolt prints UTC without zone: `05:47:20.605` = bd's `08:47:20.605+03:00`). Committer values are the Dolt commit committer (`beads`, `root`), as upstream prints them.

## Decision

### 1. Reader

T019 reads history, events and every table bd export does not cover with the **Dolt CLI v2.2.4**, on the same store copy it already makes. v2.2.4 is the first Dolt release that contains the engine commit bd 1.3.1 pins (`github.com/dolthub/dolt/go v0.40.5-0.20260806213044-796d07497741`; GitHub compare: v2.2.3 is behind it, v2.2.4 ahead by 42). Installed by CTO 2026-10-26: official release zip `dolt-windows-amd64.zip`, sha256 `d093f2fa31fa36dd3286c5c380e24693b5c3ec6e75efc50f1123f3852c8e67e5` (matches the release asset digest), unzipped to `E:/Tools/dolt-2.2.4/dolt-windows-amd64/bin/dolt.exe`, not on PATH. Apache-2.0; a build-time tool, never shipped with the fork.

Rules for every call:

- Only on a copy; never on a live `.beads/`. `cp` skips the busy `noms/LOCK` file; that is expected.
- Path from `BD_LITE_DOLT` (else the fixed path above); started by full path with `hideWindow` (existing `extract_proc_windows.go`).
- `DOLT_ROOT_PATH` = a folder inside the extractor's work dir, with `.dolt/config_global.json` written before the first call as `{"metrics.disabled":"true","versioncheck.disabled":"true"}`. Dolt sends usage metrics and checks for new versions by default; company rule is no telemetry. The owner's `~/.dolt` is never read or written.
- One `dolt sql -r json` process per extraction, statements fed on stdin; the output is one `{"rows":[...]}` document per statement, read with one `json.Decoder`.

`bd export` and `bd config list --json` stay (2 bd calls, 39 s on whisper-flow) as the source of issues/comments/deps/labels/config: the loader and verifier already consume that JSONL, and it keeps the upstream format that the round trip (T021) needs.

### 2. What is read, in one batch

1. `SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'` and `SELECT COUNT(*)` for each: the uncovered-table report is complete by construction (no hard-coded list).
2. Issue versions: `SELECT d.*, l.committer FROM dolt_diff_issues d JOIN dolt_log l ON l.commit_hash = d.to_commit ORDER BY d.to_id, d.to_commit_date`, plus the `from_*` side of `removed` rows (none today).
3. `SELECT * FROM events ORDER BY issue_id, created_at, id`.
4. `SELECT * FROM <t>` for every other non-empty base table that `bd export`/`config list` do not carry: today `local_metadata`, `leases`. These go into the loader's existing `Input.Tables` (T020 already has it).

### 3. History = one version per change, not one per commit

`Beads[i].Snapshots` keeps its current shape (upstream `HistoryEntry` JSON array, newest first, `{CommitHash, Committer, CommitDate, Issue}`), built from the `dolt_diff_issues` `to_*` columns: **one entry per commit that changed the bead**, which is exactly what `contracts/history.md` specifies ("one element per write command that changed the bead, plus the migrated Dolt versions"). Upstream's extra rows repeat an unchanged state at later commits that touched other beads (whisper-flow: 46,105 rows, 285 distinct states).

This is not data loss: every distinct state of every bead, with the commit, committer and date at which it appeared, is carried. `askedSinceFromHistory` (oldest version of the current owner-ask run) is unchanged by construction, because a run always starts at a commit where status or assignee changed, which is a diff row; T016 checks it on every rayeh bead. What is not copied is Dolt's commit graph (which unrelated commits a state survived), which beads-lite has no use for; the pre-switch Dolt store is archived untouched (ADR 0001, exit path), so it stays available. Materialising every per-commit row instead would put ~10 M rows (~5 GB at the measured 535 B of scalars per row) into company's `lite_issue_versions`, against SC-008's 100 MB.

Times are converted to UTC RFC 3339 by the extractor (Dolt CLI prints UTC with no zone); the loader keeps ADR 0001's second precision.

### 4. Events

`Beads[i].Events` is built from the `events` rows of that issue, in upstream `types.Event` JSON shape (same field names as `bd history --events --json`).

### 5. Freeze-time run and delta

The whole extraction is one copy + one Dolt batch + 2 bd calls: company ~3-4 min (59.6 s + 72.9 s + bd export/config, ~40 s on whisper-flow; T019 measures it on company). That fits inside the freeze, so the extractor runs **once, at the freeze, on a copy of the frozen store**. The "N parallel copies before the freeze, then delta of beads whose `updated_at` moved" machinery is dropped (it existed only because per-bead history took hours). A rehearsal run before the freeze is still made, to catch errors early; its output is thrown away.

### 6. Upstream bd as an oracle, on a sample

To prove the Dolt read matches what upstream prints, the extractor's test (and one run per corpus) compares, for up to 20 beads per corpus (the 10 with most versions plus 10 random), `bd history <id> --json` and `--events --json` against the Dolt-built entries after collapsing upstream's consecutive identical states. Any difference fails T019. Cost: about 40 bd calls per corpus, outside the freeze.

## Alternatives rejected

1. **Keep per-bead upstream bd calls (status quo, optionally more workers).** Measured 1,091 s for 96 beads; company has 1,686 beads x 2 calls, each scanning a `dolt_history_issues` that is ~200 times larger than whisper-flow's. At whisper-flow's per-call cost the floor is 3,372 calls x 15 s / 4 workers = 3.5 h, and each call scans a larger view, so the real figure is higher; and more workers mean more 928 MB copies and more CPU on a shared PC. It also leaves `local_metadata`/`leases` unreadable, which stops the switch. There is no batching: `history` takes one id, and no bd command takes a list.
2. **A one-off CGO build that opens embedded Dolt in-process** (July's table copier `2c0eb87bc`, or a small Go tool over `internal/storage/embeddeddolt`). Needs a C toolchain install (MinGW or zig) plus a CGO build of Dolt (the no-CGO cold build alone took 36 min); same data as the Dolt CLI, more install and more code to keep.
3. **bd server mode (`bd dolt start` with `dolt sql-server`) so `bd sql` works.** Needs the same Dolt CLI, plus changes the copy's mode and config; a running server adds a process and a port for no gain over direct `dolt sql`.
4. **Accept losing pre-migration history.** Loses rayeh's "asked since" clocks (deploy gates) and every bead's who/when; not needed once the Dolt read takes about a minute.

## Cost

$0 per active user and $0 per idle user: a one-off local migration tool, no service, no quota, no process after the switch. Install: one 40 MB zip on local disk. Machine time: ~3-4 min per company switch inside the freeze, ~1 min of Dolt for smaller corpora, instead of hours.

## Exit path

The Dolt CLI is needed only until the last project has switched; then delete `E:/Tools/dolt-2.2.4`. If a later Dolt store version is ever unreadable by 2.2.4 (it is not: both corpora read today), use the Dolt release that contains whatever engine commit the then-upstream bd pins (same compare method). The fallback if the Dolt CLI were unusable is alternative 2 (in-process CGO read), not alternative 1.

## Effect on tasks

- T019: replace per-bead history/events/provenance calls with the Dolt batch above; keep `bd export` + `config list`; drop multi-copy workers and `Delta`; add the 20-bead oracle comparison; record per corpus `copy_ms`, `dolt_ms`, `export_ms`, `config_ms`, `oracle_ms` in `bench/results/migration-extract.json` for all five corpora.
- T020: no interface change (`Beads[].Snapshots`, `Beads[].Events`, `Input.Tables` stay); `CheckUncovered` now gets real counts for every base table, and `local_metadata`/`leases` arrive as rows instead of "unreadable". The verifier counts versions per bead against the Dolt diff count, not against upstream `bd history` length.
