# Runbook: switch one project from upstream bd (embedded Dolt) to beads-lite

Owner: SRE. Contract: `specs/001-fast-compact-tracker/contracts/switch.md`. Spec: FR-017, SC-012, US8.
Order of projects: rayeh, fatura, abduu-tasks, then the rest (company, whisper-flow, ...). One project per window; never two at once.
Every step is run by full path, windowsHide, on the real project only after the same steps passed on a copy (quickstart 7, T024). Process stops use recorded PIDs only.

## 0. Preconditions (all must be true, else stop)

- G2 passed: 0 parity diffs on all five corpora; T013 timings met. Fork `bd.exe` built from the `lite` commit being shipped; its path and `git rev-parse HEAD` written on the project's switch bead.
- Upstream bd 1.3.1 still installed (passthrough target and rollback path). Backup is fresh: `CompanyBackup` ran in the last hour and `git ls-remote` shows the project's `refs/backup/beads` moved.
- One switch bead per project (`bd create "switch <project> to beads-lite" -a sre`); this runbook's step results go there as comments. No other bead or agent changes the callers' config while it is open (lesson company-kzlt).

## 1. Find active sessions (T-30 min)

A session counts as active if a Claude Code transcript changed in the last 2 h. Transcripts live in `C:\Users\abduu\.claude\projects\<folder-slug>\*.jsonl`, one folder per working directory (slug = path with `:` `\` `/` replaced by `-`). Include the project folder AND its worktrees (`git worktree list` in the project).

```powershell
$since = (Get-Date).AddHours(-2)
git -C <project> worktree list --porcelain | Select-String '^worktree ' | ForEach-Object { ($_ -replace '^worktree ','') } |
  ForEach-Object { $slug = ($_ -replace '[:\\/ ]','-'); Get-ChildItem "$env:USERPROFILE\.claude\projects\$slug*" -Filter *.jsonl -ErrorAction SilentlyContinue |
  Where-Object LastWriteTime -gt $since | Select-Object FullName, LastWriteTime }
```

Record the list on the switch bead (session id = file name, last write). `bd lite-switch announce` writes the same list into `.beads/lite-switch.json` once W13 exists.

## 2. Announce (T-30 min)

1. `bd lite-switch announce --at HH:MM` in the project (fork binary). From now every fork command prints the notice on stderr; writes still work.
2. Comment on the company bead for the migration (`~/company`, CoS-owned) asking CoS to post the notice to the sessions on the list: freeze time, expected length (target < 10 min), what they see (writes refused, exit 3), what to do (stop writing, finish the current command, `bd lite-switch ack`).
3. Wait for acks. Freeze happens only when every listed session acked or has made no write for 15 min (`bd lite-switch status`). A session that neither acked nor went quiet: move the window, do not force.

## 3. Callers (what must be repointed) - inventory

Found in `~/company` 2026-10-26; none uses `BD_BIN` or `PATH` for bd except the board launcher, so the one-time change is a repoint, not an env var.

| Caller | How it finds bd | Action at flip | Restart |
|---|---|---|---|
| agents / shells | `bd` on PATH = `%APPDATA%\npm\bd.cmd` -> `@beads/bd/bin/bd.exe` | PATH or `bd.cmd` shim points to the fork wrapper (passthrough sends Dolt projects to upstream, so one binary serves both states) | new shells only |
| git hook `scripts/commit-bead-hook.js` | `bd` on PATH | covered by the shim | none |
| learner `scripts/learner/lib/beads.mjs` | node + `%APPDATA%\npm\node_modules\@beads\bd\bin\bd.js` (hardcoded) | repoint to the shim path (swe change if it must read `BD_BIN`) | learner task |
| link-inbox `scripts/link-inbox/process.mjs` (same bd.js path) and `check-promotion.mjs` (`bd.cmd`) | as above | as above | its scheduled task |
| decision-inbox `scripts/decision-inbox/inbox.mjs` / Telegram bridge (`learner/lib/bridge.mjs`) | via learner lib | follows learner | bridge process |
| backup `scripts/backup-projects.sh` | `bd export -o` (read, 60 s limit) | works on fork (`export` is on the surface); verify one run after flip | none |
| board `scripts/start-beads-ui.sh` | `BD_BIN=@beads/bd/bin/bd.exe` | set `BD_BIN` to the fork exe (or the new board, T-US7) | `register-beads-ui-task.ps1` task |

Rule: do the shim repoint ONCE, before the first project (rayeh), while every project is still on Dolt (passthrough makes it a no-op for them); verify with the whole parity smoke. After that a project switch changes only its `metadata.json`.

## 4. Freeze (T-0)

1. `bd lite-switch status` - all acked or quiet. If not, stop.
2. `bd lite-switch freeze`. Check: a fork write exits 3 with the freeze message; upstream `bd` write fails on `.beads/embeddeddolt/.lock`; reads work on both.
3. Stop the project's long-running callers by recorded PID (board, bridge); they must not write.

## 5. Migrate and verify

`bd lite-switch migrate` -> new `beads.db`; the verify report must show 0 differences (beads, comments, deps, labels, events, config; counts and content). Any difference: go to Rollback, file a bead with the report.
Then run the 5-command smoke on the new store (show, list, ready, comments, count) and compare counts with upstream's last read (`bd count` taken at step 4).

## 6. Flip

`bd lite-switch flip` (metadata backend -> `sqlite`, state file removed, lock released). Apply the project row of step 3 (restarts in this order: board, Telegram bridge/learner, link-inbox, backup task). Comment on the bead: time frozen, time unfrozen, counts before/after, verify report path.

## 7. Project docs to update (same day)

In the project's `AGENTS.md`/`CLAUDE.md` and runbooks: remove `bd dolt push`/`bd dolt pull` and `bd doctor --fix` (they now print the replacement and exit non-zero); use `bd backup snapshot` and `bd doctor`. `grep -rn "dolt push\|doctor --fix"` in the project, its `.claude/` and `scripts/`. The backup list (`scripts/backup-projects.list`) is unchanged.

## 8. 24 h watch (checks at +1 h, +6 h, +24 h; each logged on the bead)

- p95 of `show`/`list`/`ready` from a 20-call sample <= SC-001; one create + update + comment + close succeeds.
- Fork stderr/log: zero "not supported in beads-lite" hits from our own scripts (`grep` the learner and link-inbox logs).
- Next `CompanyBackup` run: `refs/backup/beads` moved, export non-empty, count equals `bd count`.
- `beads.db` size and WAL size sane (SC-008: < 100 MB); `bd doctor` clean.
- Board: opens within SC-002, shows a change within SC-003.
- No session reports a lost write. A single failed check = decide Rollback within 1 h.

## 9. Rollback (any time before the 30-day archive expires)

`bd lite-switch rollback`: backend back to `dolt`, new `beads.db` moved to the archive, state removed; `embeddeddolt/` was never touched. Writes made on the fork after the flip are NOT in Dolt: before rolling back after a flip, `bd export` the fork store and `bd import` it into upstream (upsert), then compare counts. Repoint callers per step 3 only if the shim was changed (it is not, normally). Comment on the bead with the reason.

## 10. After

Keep `embeddeddolt/` 30 days, then delete only on explicit owner approval on the switch bead. Update the project's status in `specs/001-fast-compact-tracker/tasks.md`.
