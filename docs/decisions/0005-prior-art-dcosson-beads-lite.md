# ADR 0005: Prior art: dcosson/beads-lite and other beads replacements

- Status: accepted (CTO, 2026-10-26). Bead: company-1bo5.1 (child of company-1bo5).
- Owner decision this works under (2026-10-26, chat, verbatim): «كمّل شغلنا وقيّم الأداة الجاهزة في نفس الوقت». The fork work went on; this ADR is the parallel evaluation.
- Decision in one line: **keep our fork.** Do not adopt dcosson/beads-lite, do not move our work into it. Rename our fork before anything public (section 6). Do not run the team on Gas Town or Gas City; borrow two ideas as a dispatch rule and a script (section 7). Nothing in the fork plan changes.

## Context

On 2026-10-26 the owner found https://github.com/dcosson/beads-lite ("a lightweight, drop-in replacement for beads", Go, MIT, one JSON file per issue, built for Gas Town). We had already forked gastownhall/beads 1.3.1 into our own "beads-lite" (SQLite instead of embedded Dolt, ADR 0001). Our research (`specs/001-fast-compact-tracker/research.md`) compared only "fix upstream / fork upstream / write our own"; it never searched for existing replacements. That was a miss: the sweep below should have come before the fork plan. It did not change the answer, but we could not have known that.

## 1. Prior-art sweep (done 2026-10-26)

Queries (GitHub `gh search repos`, each `--limit 10-15`): `beads-lite`, `beads_rust`, `beads-rs`, `beads issue tracker`, `beads compatible`, `steveyegge beads`, `gastown beads`, `beads alternative`, `beads viewer`, `beads mcp`, `beads jsonl`, `beads sqlite`, `beads fork`, `fast beads`, `lightweight beads`, `drop-in replacement beads`, `replacement for bd`, `bd beads replacement`, `beads go sqlite issue tracker`; forks of dcosson/beads-lite (`gh api repos/dcosson/beads-lite/forks`: none); npm `npm search beads`. Go packages: none beyond the GitHub repos (Go modules are GitHub repos; no separate registry hit). crates.io was covered through the GitHub hits (beads_rust, minibeads are published crates).

Results that are trackers (not viewers/plugins), as of 2026-10-26:

| Project | Lang / storage | Stars | Last push | Command / surface | Relevance |
|---|---|---|---|---|---|
| Dicklesworthstone/beads_rust | Rust, SQLite + JSONL | 1136 | 2026-10-24 | `br`, own JSON (`.issues[]` envelope) | Most used replacement. Not `bd`-compatible by name or JSON. Licence: MIT **plus a rider that grants no rights to OpenAI, Anthropic or anyone acting for them, and counts "executing, benchmarking, testing" in automated pipelines as use** (see section 5). Not run. |
| dcosson/beads-lite | Go, one JSON file per issue | 12 | 2026-08-11 | `bd`, targets bd 0.49/0.50 | The tool the owner found. Evaluated in full below. |
| philcunliffe/beads-lite | Go, SQLite-only fork of gastownhall/beads (modernc.org/sqlite, CGO off) | 1 | 2026-05-15 | `bd-lite`, upstream 1.0.4 | Same idea as ours, on an older base (1.0.4, we are on 1.3.1). 100 commits over two days, then idle. A source to read for the SQLite store, not to adopt. |
| kylesnowschwartz/beads-lite | Go | 2 | 2026-05-03 | own | Minimal tracker, not a drop-in. |
| fxcl/beads-lite | Rust | 0 | 2026-09-28 | own | No description, no README claim of compatibility. |
| rrnewton/minibeads | Rust, markdown files | 16 | 2026-10-03 | `mb` (alias to `bd`), MCP-compatible | "WIP"; markdown storage; targets the old steveyegge MCP surface, not bd 1.3 JSON. |
| jedarden/bead-forge | Rust | 4 | 2026-08-24 | drop-in for `br` | Replacement of beads_rust, not of bd. |
| bpcakes/beadroll | Rust, git ref journal | 1 | 2026-10-26 | `bead`, reads/writes Beads JSONL | Not a fork; multi-machine sync design. Too new (created 2026-10-07). |
| delightful-ai/beads-rs, fwindolf/beads-rs | Rust | 24 / 1 | 2026-08 / 2026-02 | own | Distributed / rewrite; not bd-compatible. |
| scgoetsch/cairn, ariesworx/aries, evangstav/fmw | Python / Go (Dolt) / Python | 0-3 | 2026-10 | own | Read-only shadow, AGPL Dolt-backed, or single-JSONL toy. |

Viewers and boards found on the way (beads_viewer 1721 stars, perles, bdui, beads-web, beadee, `beads-ui` on npm, `@seungyeop-lee/beads-ui`, view-beads, ...) all drive `bd` or read its store; none fixes the store or command latency, which is our problem. They stay candidates for the board only if our own board redesign stalls.

Upstream itself: gastownhall/beads, 28,212 stars, pushed 2026-10-26; `@beads/bd` 1.3.2 on npm. Upstream is alive, so our fork must keep tracking it (as planned).

## 2. dcosson/beads-lite on our needs

Setup: cloned 2026-10-26 at `b8779e93e` into a scratch folder outside every repo; built with `E:\Tools\go1.26.5` (`GOTOOLCHAIN=local`, `CGO_ENABLED=0`). Data: the five pristine corpus stores (`E:/Projects/beads-lite-corpus/stores-pristine/*/.beads/issues.jsonl`, the same snapshot the upstream-1.3.1 oracle was captured on), converted into its file layout by a script, because its `import` is a no-op. Replay: every command of the upstream oracle (`oracle/upstream-1.3.1/*/_index.jsonl`, 861 commands over five stores, verbatim arguments) run against it and compared. Write rows probed by hand on a copy. Scripts and raw results are kept in the CTO session scratch folder, not in this repo (they contain real bead data).

### 2.1 It does not build on Windows

`go build ./cmd` fails: `syscall.Flock` (Unix only) in `internal/issuestorage/filesystem/filesystem.go` (7 calls) and `internal/config/yamlstore/yamlstore.go` (2). No CI target or note for Windows. A 40-line local shim (`LockFileEx` on a far byte range) made it build; everything below ran on that shim. This is the cheapest gap to close and the only one where a contribution upstream would be natural.

### 2.2 Compatibility, row by row (contracts/cli-compat.md)

Whole oracle replay: 861 commands; same exit code on 549 (64%); of 708 JSON commands, 427 produced parseable JSON (60%), 241 had no field missing versus upstream (34%); of 406 commands that return ids, 123 returned the same id set (30%; comment rows fail only on id type, see row 3).

| Row | Command | Result | Evidence |
|---|---|---|---|
| 1 | `show --json [--include-dependents]` | **fail** | Ids right (56/56) but 156/156 JSON outputs miss fields: `dependency_count`, `dependent_count`, `comment_count`, `comments_omitted`, `revision`, `started_at`, plus `design`, `acceptance_criteria`, `notes`, `metadata` (not in its data model). `--include-dependents`: unknown flag. |
| 2 | `update --claim -s -a --design --set-metadata --if-status --if-assignee` | **fail** | `--claim`, `-s`, `-a` work. `--design`, `--set-metadata`, `--if-status`, `--if-assignee`: unknown flag. Worse: a plain `update -a` **rewrites the file without `metadata` and `design`** (Go struct has no such fields), so the first write by any agent silently deletes decision cards (`card_*`) and `title_ar`. Claim race: correct, exactly one winner (2.4). |
| 3 | `comments --json`; `comments add -f/-a` | partial | Author and text equal on 40/40 sampled beads (fatura). Comment `id` is an integer per bead, ours are UUIDs: 6,426 company comment ids would change. Arabic text round-trips. Times written with local offset, not UTC. |
| 4, 17 | `list --status --assignee -l --all --json --limit 0 --tree=false` | **fail** | `--tree=false` unknown (the board's main call). Default `--limit 50` versus upstream's; `--flat`, `--brief`, `--reverse`, `--ready`, `--id`, `--has-metadata-key`, `--desc-contains`, `--no-*`: unknown. Same id set on 43/78 list commands. |
| 5 | `create --silent -a -p --parent --body-file` | **fail** | `--silent`, `--body-file`: unknown flag. |
| 6 | `close --reason --force` | partial | `--reason` works, `--force` unknown. |
| 7, 19 | `ready` | **fail** | Different meaning: 0/15 ready commands return upstream's set (company: 185 vs 100 ids, 152 not in upstream's). `--explain`, `--brief`: unknown. |
| 8 | `blocked`; `dep add/remove` | **fail** on blocked | `blocked` 0/5 equal sets (company 66 vs 123). `dep list` 24/24 equal ids. |
| 9 | `label add` | pass | `label list` values equal (shape differs: array of strings). |
| 10, 16 | `export -o`, `export --events` | **fail** | No `export` command. `import` is a documented no-op. There is no way to get our data out in bd JSONL; our backup script and exit path depend on it. |
| 11 | `config get/set` | partial | Works; our `status.custom` syntax `owner_decision:wip` (bd 1.3) is rejected, plain names work. |
| 12, 21 | `init --non-interactive`, `bootstrap --yes` | **fail** | `--non-interactive` unknown; no `bootstrap`. Store is JSON files, not `beads.db`. |
| 13 | `count`, `import`, `bootstrap` | **fail** | No `count` (66/66 fail), `import` no-op, no `bootstrap`. |
| 14 | `--actor`, `BD_ACTOR`, `BEADS_ACTOR` | partial | Env vars honoured; global `--actor` flag unknown (only on `create`). Every link-inbox call passes `--actor`. |
| 15 | `history <id> --json`, `--events` | **fail** | No `history` command and no change history at all. rayeh's deploy gates (`tools/deploy/gate-lib.mjs`) read it. |
| 18 | `search` | pass (shape) | Works, returns JSON. |
| 20 | `update --status` | pass | |
| 22 | `dolt push/pull`, `doctor --fix` | n/a | No `dolt`; `doctor` exists. |

Custom statuses: supported via `status.custom` (plain names only, no `:wip` category, so the board's category grouping is lost). Labels, deps, parent/child: supported. Fields carried by our stores and absent from its model, beads affected across the five stores: `metadata` 385, `design` 328, `acceptance_criteria` 82, `notes` 29, `external_ref`/`source_system` 158, `started_at` 1,880, `lease_expires_at`/`heartbeat_at` 138.

### 2.3 beads-ui / our board

The board's refresh (`contracts/board-data.md`) is `list --json --tree=false --limit 0`, `ready --json --limit 1000`, `blocked --json`, `show --json --include-dependents`. Two of the four are refused (`--tree`, `--include-dependents`), the other two return different sets. Neither stock beads-ui nor our board works on it without changes in both.

### 2.4 Timings and concurrency (company copy: 1,619 beads, 645 open, largest open bead 212 KB)

The PC was heavily loaded by other agents during the whole run (upstream bd's own baseline that morning: `version` 1.5 s median, `show` 15.9 s). Its own `version` (process start, no store work) is given as the floor.

Medians in ms over three sequential passes (n = 5, 5, 10; the first pass had output capped at 1 MiB, so its big-output rows are left out). The spread between passes is machine load, not the tool; read the floor row first.

| Command | Pass A | Pass B | Pass C | Target (SC-001 / SC-005) | Output |
|---|---|---|---|---|---|
| `version` (floor) | 580 | 135 | 569 | | 31 B |
| `show <largest> --json` | 6,879 | 688 | 705 | 500 | 214 KB (SC-006: 8 KB) |
| `list` (default text) | 346 | 2,872 | 8,315 | 500 | 53 lines |
| `list --json --limit 0` (open) | | 324 | 3,228 | 500 | 1.3 MB |
| `ready --json` | 1,977 | 631 | 2,714 | 500 | 63 KB |
| `blocked --json` | 4,246 | 990 | 1,760 | 500 | 27 KB |
| `comments <largest> --json` | 262 | 175 | 373 | 500 | 211 KB |
| `update -a` | 247 | 400 | 9,249 | 500 | |
| `comments add` | 248 | 380 | 293 | 500 | |
| `create` | 220 | 245 | 1,035 | 500 | |

On the quietest pass (B) writes, `comments` and `list --json` meet the 0.5 s target; `show` of a well-connected bead, `ready` and `blocked` do not (0.6-1.0 s with a 0.135 s floor), and `show` output is 27 times the SC-006 limit. Upstream bd 1.3.1 on the same PC that morning: `show` 15.9 s, `list` 13.2 s, `ready` 13.5 s, `update` 25.4 s median (SRE baseline `bench/results/baseline-2026-10.json`), so it is far faster than upstream, but not inside our targets. Our fork's numbers come from W6/bench on the same harness; they are not in yet.

Concurrency (company copy, each worker: create, 3 comments on one shared bead, a status update, `list --json`, `show`; plus one claim race on a shared bead):

| Workers x rounds | Calls | Failures | Lost writes | Claim winners | p50 | p95 | max | Calls over 2 s |
|---|---|---|---|---|---|---|---|---|
| 10 x 3 (small store, whisper-flow) | 220 | 9, all "already claimed" (correct refusals) | 0 (30/30 creates, 90/90 comments, 0 status lost) | 1 | 2.2 s | 7.5 s | 11.4 s | 123 |
| 5 x 3 (company) | 110 | 4, all correct claim refusals | 0 (15/15, 45/45, 0) | 1 | 1.1 s | 3.4 s | 4.8 s | 21 |
| 20 x 3 (company) | 440 | 19, all correct claim refusals | 0 (60/60, 180/180, 0) | 1 | 7.7 s | 32.0 s | 58.9 s | 408 |

Correctness under contention is good: no lost write and exactly one claim winner at every level. Latency is not: SC-004 (no call over 2 s with 10 agents) fails, mostly because every `list` reads every file and every comment on a hot bead rewrites the whole file under its lock.

What it gets right: per-issue file locks, no global lock, atomic temp-file-plus-rename writes. Writes are fast and correct under contention. What it gets wrong for us: anything that walks the graph (`show` of a bead with many relations, `ready`, `blocked`) opens one file per related bead, so time grows with relations and with the antivirus cost of each open on Windows; and default `show` prints the whole bead (214 KB for the largest open one), which fails SC-006 (8 KB).

### 2.5 Telemetry, maintenance, licence

- Telemetry: none. No `net/http` import anywhere; no network calls.
- Maintenance: one author (Danny Cosson; the other committer names are his agents, same e-mail). 338 commits: 313 in January-February 2026, 22 in March, 3 since (July 3, August 11 x2). No releases, 0 issues, 0 pull requests, 0 forks, 12 stars. `bd version` reports 0.50.0; the gap to bd 1.3.1 (our callers' surface) is the whole 1.x line.
- Licence: MIT. No obstacle to reading or borrowing.

## 3. Decision

**Keep our fork; do not adopt dcosson/beads-lite; do not contribute our work to it.**

Adopting it means rebuilding most of our compatibility surface inside a codebase whose data model has no field for decision cards: 14 of the 22 contract rows fail, the first agent write would silently delete metadata on 385 beads, there is no export (no exit path, no backup) and no history (rayeh gates break). Its one real win over upstream (no global lock) is something our SQLite WAL design already gives us (ADR 0001, research R3).

Keep / drop of our fork: nothing to drop. Our board redesign, the freeze/switch tool, compact output (SC-006), history, export and migration all have no counterpart there.

What we borrow (ideas only, no code needed): (a) its benchmark harness shape (same scenario against reference `bd` and the replacement, side by side) is what our `bench/` already does; (b) its per-issue lock confirms our claim-race test should assert "exactly one winner" under 20 writers, which it does.

Contribute? Only the Windows lock shim would be worth offering (a small, self-contained PR). That is a public action under the owner's GitHub identity, so it is optional and the owner's call; it is not filed as work.

### Alternatives rejected

1. **Adopt dcosson/beads-lite and add what we need.** Rejected: 14/22 rows fail, no metadata model, no export/history, no Windows build, dormant upstream (3 commits in 7 months). We would own a second, older surface (0.50) and still have to write everything our fork is writing.
2. **Switch to beads_rust (`br`).** Rejected: different command name and JSON shapes, so every caller changes (SC-009 says no caller changes); and its licence rider withholds all rights from Anthropic and anyone acting for it and counts execution in automated pipelines as use, while every one of our callers is a Claude agent (section 5). Not run for that reason.
3. **Restart from philcunliffe/beads-lite (SQLite fork of upstream 1.0.4).** Rejected as a base: three minor versions behind our 1.3.1 base, idle since May, no history/export work. Kept as reading material for W2 if a SQLite query question comes up.

### Cost

Unchanged from the fork plan: $0 per active user, $0 per idle user (local CLI, no service, no process when idle; dcosson/beads-lite would be the same). Quota hit first: none (no provider). Exit path: our fork exports bd 1.3.1 JSONL that stock `bd import` accepts (contract row 10); dcosson/beads-lite has no export at all, which alone disqualifies it under the portability lens.

## 4. What changes in our plan

Nothing in the task list. One addition to the research file (owner of `research.md`: CTO): a "Prior art" section pointing here, so the next reader sees the sweep was done.

## 5. Licence note (suggestion, not a block)

beads_rust's LICENSE is MIT with an "OpenAI / Anthropic rider": no rights to those companies "or any person or entity acting directly or indirectly on behalf of, for the benefit of, or under the direction of" them, and "use" includes "executing, benchmarking, testing ... in any ... pipeline for machine learning or other automated systems". Whether a Claude agent run by us falls under it is a legal reading; the technical verdict (alternative 2) does not depend on it. Recorded so nobody installs `br` for agents without knowing.

## 6. Name clash

"beads-lite" is taken at least four times on GitHub: dcosson/beads-lite (Jan 2026, binary also `bd`, 12 stars), philcunliffe/beads-lite (May 2026, same idea as ours), kylesnowschwartz/beads-lite, fxcl/beads-lite. Ours (abdelrahman-atef1/beads-lite) is public. Inside the company the name is harmless; in public it is confusing, and dcosson's tool ships a `bd` binary too. Recommendation: rename the repo and product before any public release or announcement; the binary stays `bd` (drop-in is the point). Choosing the name is a product/owner decision, filed as [company-1bo5.2](http://127.0.0.1:3000/#/issues?issue=company-1bo5.2) for the pm role. Rename cost now: repo rename on GitHub (redirects kept), `SETUP-NOTES.md`, backup list entry, spec titles; no code paths depend on the name.

## 7. Gas Town and Gas City as our agent-team runner (added 2026-10-26, owner asked)

Question (owner, 2026-10-26, via CoS): should Codesters run its agent team on Gas Town (gastownhall/gastown) or Gas City (gastownhall/gascity, including its `GC_BEADS=file` provider) instead of what we hand-built (CoS as coordinator, role agents, worktrees, `~/company/locks`, beads), or borrow ideas from them? Read from their READMEs, install guides, release assets and source on GitHub, 2026-10-26. **Nothing was installed or run**: both need tools this PC does not have (below), and installing them changes the machine.

| | Gas Town (`gt`) | Gas City (`gc`) |
|---|---|---|
| What | Workspace manager: Mayor (coordinator AI), per-project Witness (stuck-agent monitor) and Refinery (merge queue), Deacon (supervisor AI on continuous patrol), Polecats (workers), convoys, escalation, scheduler, Seance | SDK that extracts Gas Town's parts into a declarative `city.toml`: runtime providers, work routing, formulas, health patrol, controller loop |
| Licence, activity | MIT; 18,546 stars, 1,688 forks; last push 2026-10-25; releases to v1.2.1 (2026-06-06) | MIT; 1,442 stars, 451 forks; last push 2026-10-26; v1.5.1 (2026-10-24) |
| Size | 14.6 MB of Go source | 90.4 MB of Go source (beads itself: 45.3 MB) |
| Windows | Windows zip is published, but the README says: "For full tmux-backed workflows on Windows, use WSL or another Linux environment. Native Windows shells are best treated as minimal CLI-only environments." Mayor, Witnesses, Refineries and polecats are tmux sessions. ICU builds need MSYS2. | No Windows build at all (release assets: darwin and linux only). tmux, jq, pgrep, lsof required "always"; tmux stays required even with another session backend. |
| Tracker / Dolt | Needs `bd` 0.57+ **and Dolt** (`gt up` boots Dolt); Wasteland federation goes through DoltHub | Default provider needs `bd` 1.3.0, Dolt 2.1.0+ and `flock`. `GC_BEADS=file` drops all three, but it is Gas City's own internal store: one JSON file for the whole store (`internal/beads/filestore.go`: `{seq, beads[], deps[], revisions, fences}`) rewritten on every write, not a `bd` CLI. Our callers, board, decision cards and 14 MB stores would not move onto it. |
| Always-on cost | `gt up` starts long-lived services; the Deacon is an AI agent on "continuous patrol", the daemon heartbeats every 3 min, one Witness and one Refinery session per project. Those are model sessions spending tokens while no work is waiting. | Controller/supervisor loop reconciling desired state; health patrol |
| Telemetry | OTLP logs and metrics (session starts, every bd call, spawns) to a backend set by env vars | not checked |

Verdict: **adopt neither.**
- **Windows**: the full system needs WSL (or Docker) for tmux. Our work is Windows-native: Flutter and Android builds, `adb` to the shared Redmi, PowerShell scripts, Task Scheduler jobs with `conhost --headless`, the antivirus rules in `lessons.md`. Running the team inside WSL splits it from the tools it drives; running it native leaves only the "minimal CLI-only" part.
- **Dolt**: both default to the bd + Dolt stack that company-1bo5 is removing because of its latency and 562 MB store growth. Gas City's file provider avoids Dolt but rewrites one file per write and is not `bd`.
- **Cost**: tool cost is $0, but the model is always-on AI supervisors (Mayor, Deacon, Witness and Refinery per project). That is token spend when nobody is working, against our zero-idle-cost value, and more load on a PC that is already the bottleneck (section 2.4).
- **Boring technology**: 14.6 MB (Gas Town) or 90 MB (Gas City) of fast-moving Go plus tmux, Dolt, ICU and WSL, against our few hundred lines of runbooks, locks and skills. Exit cost would be high: roles, mail, convoys and formulas are their own concepts.

Borrow, as plain scripts or rules, not services:

| Their part | Ours today | Borrow? |
|---|---|---|
| Scheduler (`scheduler.max_polecats`, capacity-governed dispatch) | No cap; CoS starts helpers freely. This run measured the cost: `bd version` alone took 0.13-0.57 s median on the loaded PC | **Yes.** [company-1bo5.3](http://127.0.0.1:3000/#/issues?issue=company-1bo5.3) (cos): a measured cap on concurrent helpers |
| Witness (stuck-agent detection and recovery) | `issue-triage` skill, run by hand | **Yes, without an AI session.** [company-1bo5.4](http://127.0.0.1:3000/#/issues?issue=company-1bo5.4) (sre): hourly read-only script that flags stale claims (bd already has `heartbeat_at`, `lease_expires_at`) |
| Refinery (batched merge queue with gates) | `waiting_for_merge` status and `knowledge/runbooks/landing.md` | Not now. Our merge volume is a few branches a day; a queue process is more than we need. Revisit if landings collide. |
| Seance (ask a predecessor session) | Rule: state and next step are written on the bead before a run stops | No. Bead-first handover is cheaper and survives a lost session; a session id on the bead is enough if needed. |
| Escalation (severity-routed `gt escalate`) | `owner_decision` decision cards, Telegram | No; we already have it, with stricter rules (never-by-phone classes). |

Cost of this section's recommendation: $0 per active and idle user (no product change); the two borrowed items are a dispatch rule and one hourly read-only script.
