# Quickstart: proving 001-fast-compact-tracker end to end

Every check runs on **copies** of the stores (QA's `parity/` corpus), never on a live `.beads/`. Programs started by full path; every spawn hidden. A check that could not run is reported "not exercised", never "passed".

## Prerequisites

- Fork built: `CGO_ENABLED=0 go build -o <build dir>/bd.exe ./cmd/bd` on branch `lite` (P0.1).
- Upstream bd 1.3.1 at its install path (the oracle and the passthrough target).
- Golden corpus `parity/` (P0.2): copies of company, rayeh, fatura, abduu-tasks, whisper-flow + saved upstream outputs; 12x generator in `bench/`.

## Scenarios

| # | Story | Run | Expect |
|---|---|---|---|
| 1 | storage core | `go test ./internal/storage/sqlitedialect/... ./internal/storage/sqlitestore/...` incl. `backend/conformance.RunAll` | green; XFAIL list = the CTO-reviewed one (G1) |
| 2 | US3 | parity runner over every row of [cli-compat](./contracts/cli-compat.md) on each corpus copy, migrated | 0 differences (G2) |
| 3 | US3 / rayeh | rayeh gate replay: `askedSinceFromHistory` on fork vs upstream `history --json` for every rayeh bead | equal for all |
| 4 | US1 | `bench/` timings: show/list/ready/comments/count/writes, median of 20, board open + 3 agents, today's and 12x data | SC-001, SC-005 |
| 5 | US1 / US6 | `bench/` stress: 10 then 20 parallel writers x 50 writes + backup snapshot running | 0 failures, 0 lost writes, no call > 2 s (SC-004) |
| 6 | US4 | `bd lite-switch migrate` on a copy, then fork `export` -> stock bd `import` on a scratch copy -> re-export | verify report 0 differences; re-export equal (SC-007) |
| 7 | US8 | dry run of [switch](./contracts/switch.md) on a copy with a scripted session writing before and during the freeze | notice printed, write refused during freeze (fork and upstream), 0 writes lost, rollback restores (SC-012) |
| 8 | US2 | board on a migrated copy: open time, change-to-screen time, agent latency while open | SC-002, SC-003 |
| 9 | US7 | owner walkthrough of the Stitch design, then of the built board; UX compares built vs design | owner approval recorded; owner_decision on first screen; clicks <= today (SC-011) |
| 10 | US5 | `show` of the 246 KB bead and default `list` on company copy, text mode | <= 8 KB, <= 50 lines, "more" hint; `--full` = upstream text (SC-006) |
| 11 | SC-009 | learner and link-inbox suites with `BD_BIN` = fork | pass unchanged |
| 12 | SC-008 | 30 days x 3,000 simulated writes on company copy | store < 100 MB |
