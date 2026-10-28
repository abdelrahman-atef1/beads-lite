# Cutover runbook: live projects to beads-lite

Owner: SRE (sre). Written 2026-10-27 (company-1bo5). Rehearsed on COPIES only; nothing live was changed while writing it.
Scripts live in `E:/Projects/beads-lite/docs/runbooks/cutover/`: `shim.sh` (replace/restore the bd every caller uses), `project.sh` (switch one project, auto-rollback), `rehearse.sh` (dry run on a copy).

## 0. Rules for the night

- Every path is ABSOLUTE. Relative `--work-dir`, `--report` or `--dir` break (the freeze holder and the extractor run from other folders). Use forward slashes: `E:/Projects/rayeh`.
- Live projects are `E:/Projects/<name>`; the company store is `~/company` (`~/company`). Worktrees (`rayeh-*`, `*-wt-*`) are not switched: they are copies; nothing opens them after the main folder flips.
- One project at a time. The CoS tells that project's chats to stop BEFORE `project.sh`, and restarts them AFTER the OK line.
- Stop on a refused program start (EPERM, "Access is denied"): report, do not retry.
- Run each script in the background to a log and poll the log (a foreground wait of ~10 min kills the agent): `nohup bash <script> ... > <log> 2>&1 &`.
- Never `rm` anything. `embeddeddolt/` is never touched; rollback archives `beads.db` to `.beads/lite-archive/`.

## 1. Preconditions (all must be true before the first project)

1. The lite binary is built from the landed lite branch (not from a scratch branch) and is at an absolute path, e.g. `E:/Tools/bd-cutover/bd-lite.exe`. `bd-lite.exe version` runs.
2. Known defects of `origin/swe/t015-fixes db7573c4e` found in rehearsal (see section 6): the migrate step fails while frozen unless the `.lock` skip fix is in the binary; freeze did not refuse a fork write. Do not start until the binary has the `.lock` fix (branch `sre/cutover-fixes` in `E:/Projects/beads-lite-sre-cutover`, one line in `internal/migration/sqlitelite/extract.go`).
3. Disk: free space on E: at least 2x the project's `embeddeddolt` (rayeh 349 MB, fatura 278 MB, abduu-tasks 106 MB, company 610 MB, whisper-flow 53 MB, patient-record 12 MB, brother-exams 2 MB, workout-tracker 2 MB; sizes read 2026-10-27).
4. The hourly `CompanyBackup` has just run, or run it by hand: `bash ~/company/scripts/backup-projects.sh` (it reads each project once with `bd export`). Do not let it run during a freeze: it takes the same lock (company-gv1c). Pause by checking the task is not running: `schtasks /query /tn CompanyBackup`; do not edit it (one bead owns that task).
5. `BD_ACTOR=sre`.

## 2. Step A: install the lite bd once (the shim)

Every caller ends at `%APPDATA%/npm/node_modules/@beads/bd/bin/bd.js`, which spawns the `bd.exe` next to it:
the `bd` on PATH (npm shim), `scripts/learner`, `scripts/link-inbox`, the Telegram bridge, `start-beads-ui.sh` (`BD_BIN=.../@beads/bd/bin/bd.exe`) and the backup scripts. Swapping that one file moves all of them. The fork keeps the Dolt backend, so projects not yet switched keep working (passthrough). E:/Tools has no `bd` on PATH.

```
nohup bash E:/Projects/beads-lite/docs/runbooks/cutover/shim.sh status > E:/Tools/bd-cutover/shim-status.log 2>&1
nohup bash E:/Projects/beads-lite/docs/runbooks/cutover/shim.sh install E:/Tools/bd-cutover/bd-lite.exe > E:/Tools/bd-cutover/shim-install.log 2>&1 &
```

What it does: records the sha256 of upstream `bd.exe` in `bd.exe.upstream.sha`, copies it to `bd-upstream.exe` (kept; `migrate` needs it as its source), renames the live `bd.exe` away (a rename works while the board has it open; a delete would not), puts the lite binary in as `bd.exe`. A running board/bridge keeps the old binary until restarted.

ONE-COMMAND ROLLBACK (restores upstream byte for byte, verified against the recorded sha):

```
bash E:/Projects/beads-lite/docs/runbooks/cutover/shim.sh rollback
```

Rehearsed on a copy folder (`SHIM_DIR=E:/Tools/bd-cutover/shimcopy`): install 42 s, rollback 25 s on a 100%-CPU PC (dominated by sha256 of 110 MB); sha after rollback identical to before. `DRYRUN=1` prints the steps without doing them.

Smoke right after install: `bd version` (expect "(dev)"), then `cd ~/company && bd ready | head -3` (company store is still dolt: passthrough).

Also repoint the new board's `BDUI_NEW_BD` (default `E:/Tools/bd-board/bd.exe`) to the same lite binary when the board is started for the switched projects.

## 3. Step B: per project, in this order

Order: rayeh, fatura, abduu-tasks, company, whisper-flow, patient-record, brother-exams, workout-tracker.
Per-project commands (the CoS has already stopped that project's chats):

| Project | Command (run in background, log beside it) |
|---|---|
| rayeh | `bash E:/Projects/beads-lite/docs/runbooks/cutover/project.sh rayeh E:/Projects/rayeh` |
| fatura | `... project.sh fatura E:/Projects/fatura` |
| abduu-tasks | `... project.sh abduu-tasks E:/Projects/abduu-tasks` |
| company | `... project.sh company ~/company` |
| whisper-flow | `... project.sh whisper-flow E:/Projects/whisper-flow` |
| patient-record | `... project.sh patient-record E:/Projects/patient-record` |
| brother-exams | `... project.sh brother-exams E:/Projects/brother-exams` |
| workout-tracker | `... project.sh workout-tracker E:/Projects/workout-tracker` |

`project.sh` runs, with a time per step in `E:/Tools/bd-cutover/work/<name>-<stamp>/switch.log`:
`announce --at <now> --project <name> --dir <abs>`, `ack --as sre`, `freeze --force` (CoS already stopped the chats; no session can ack), `migrate-to-sqlite --bd <upstream> --work-dir <abs> --report <abs>`, `flip`, then the smoke: list, ready, create, show, update, comments add, close. Any failure runs `lite-switch rollback` and exits 1 (the log names the step; the step output is `<step>.out`). Success prints `OK <name> on sqlite`.

After the OK line, the CoS restarts that project's chats, the board and the Telegram bridge (they hold the old store).

Manual rollback of a project later (also safe after a good flip; any beads written on sqlite since are LOST from dolt, so export them first): `bd export -o E:/Tools/bd-cutover/work/<name>-post.jsonl` then `bash .../project.sh <name> <abs dir> rollback`.

## 4. After all projects

- Backups: `backup-push.sh` still calls `bd dolt push` (every 6 h) and a sqlite project has no dolt remote, so that stage will warn. `bd export` (hourly snapshot) is unaffected. File to SRE: change the backup to skip dolt push for sqlite projects (new bead, not done tonight).
- Re-enable nothing manually: `CompanyBackup` was never paused.
- Check `bd ready` in each project from a fresh shell (cold start) and record time.

## 5. Rehearsal evidence (COPIES, E:/Tools/bd-cutover/rehearsal*, 2026-10-27, PC at 100% CPU)

Measured (loaded PC; upper bounds): announce 3-23 s, ack 2-4 s, freeze 7-12 s, rollback 5 s (no beads.db yet), shim install 42 s / rollback 25 s (sha256 of 110 MB). Dolt-side smoke on the rayeh copy: list 22-39 s, show 13-33 s, ready 10-18 s, create 75-84 s.
NOT yet measured: migrate to completion, flip, post-flip smoke, rollback with a beads.db, second flip. Rehearsal 2 (rayeh) got past the lock fix into the per-bead show verification and was stopped after 53 min (about 6 s per bead on this PC, so ~100 min for 1034 beads); rehearsal 3 (whisper-flow copy, 204 beads, via project.sh itself, log E:/Tools/bd-cutover/rehearse3.log) was still migrating at 04:55. Migrate duration is dominated by 2 x N process starts, so it needs a quiet PC: stop the runaway load first (qemu-system-x86_64-headless and two find.exe held 23,000 / 13,000 CPU-seconds tonight).
Do not run the night cutover until one full run (migrate OK, flip, smoke, rollback, flip again) has passed on a copy with the final binary.

## 6. Defects found in rehearsal (not mine to fix; filed as beads for swe)

1. migrate while frozen fails: `read ...embeddeddolt\.lock: The process cannot access the file because another process has locked a portion of the file` (the holder byte-range-locks `.lock`; `copyBeads` reads it). Fix: skip `.lock` in `copyBeads`. Verified in rehearsal 2 (see bead).
2. A fork `create` ran with rc=0 while `lite-switch.json` said `frozen` (contract: exit 3).
3. Upstream `list` works during the freeze (reads do not take the lock); contract says upstream "fails on the lock" for writes only: untested here.
