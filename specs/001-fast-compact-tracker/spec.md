# Feature Specification: Fast, compact, bd-compatible local tracker and board

**Feature Branch**: `lite` (spec dir `001-fast-compact-tracker`)

**Created**: 2026-10-25

**Status**: Draft

**Input**: Owner, 2026-10-25: «عايز اداة تكون متكورة اكثر منها واسرع (ميبقاش كتر التاسكات مؤثر على السرعة)». Decisions: fork beads + beads-ui as beads-lite; «نفذ sqlite من البداية» (SQLite from the start). Evidence: company-1bo5 design (measurements, 2026-10-25), bd-surface.md (commands our scripts use), ui-calls.md (why the board is slow).

## Problem (evidence)

Measured on the owner's machine, real data (company: 1,619 beads, 6,426 comments; rayeh 1,034; fatura 536; abduu-tasks 304; whisper-flow 90):

- One `bd` command takes 2-10 s live (show 8.3-9.9 s, list 7-9.7 s, ready 10.9 s+); an idle copy still takes 0.6-2.4 s.
- The board runs about 7-10 `bd` processes one after another on every change; one refresh is 30-90 s, and agents queue behind it.
- Commands dump everything: `bd show` of one bead printed 246 KB; `bd list` 653 lines / 94 KB; `bd list --json` 2.3 MB.
- Task count is not the cause: a 90-bead project is as slow as a 1,619-bead one. Cost comes from one-process-at-a-time locking, per-command start-up, and a history store that grows with every write (about 3,000 commits a day).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agents read and write tasks without waiting (Priority: P1)

An agent (or script, hook, bridge) runs `list`, `show`, `ready`, `comments`, `create`, `update`, `close` while other agents and the board are active. Each call returns in well under a second and never queues behind another reader or the board.

**Why this priority**: This is the owner's core complaint; every agent run is slowed by it today.

**Independent Test**: Run the command set from the compat surface against a copy of the company data with 10 agents in parallel and the board open; record wall-clock per call.

**Acceptance Scenarios**:

1. **Given** today's company data, **When** an agent runs `show <id>`, `list`, `ready`, **Then** each finishes within the SC-001 limits, with the board open.
2. **Given** 10 agents issuing mixed reads and writes at once, **When** all run, **Then** none fails, none exceeds SC-004, and no write is lost.
3. **Given** a bead with a 246 KB comment history, **When** `show <id>` runs, **Then** output is at most SC-006 in size by default and says how to get the rest.

---

### User Story 2 - The board is instant and stays calm (Priority: P1)

The owner opens the board (same address and link format as today) and sees all columns, including owner_decision, waiting_for_test and waiting_for_merge. When any bead changes, the board updates quickly without freezing or taking the tracker for itself.

**Why this priority**: The board is the owner's main window and today the biggest source of slowness for everyone.

**Independent Test**: Open the board on today's data and on 12x data; time first view, time to reflect a change, and agent command latency while the board is open.

**Acceptance Scenarios**:

1. **Given** today's data, **When** the board is opened, **Then** all columns are usable within SC-002.
2. **Given** the board is open, **When** an agent updates a bead, **Then** the board shows the change within SC-003 and agent commands are not slowed.
3. **Given** a link `http://127.0.0.1:3000/#/issues?issue=<id>`, **When** opened, **Then** that bead's detail shows.
4. **Given** a very large closed column, **When** the board loads, **Then** it shows a bounded recent slice with a way to see more, not everything.

---

### User Story 3 - Existing scripts, hooks and agent prompts keep working (Priority: P1)

Every script, git hook, Telegram bridge, learner, link-inbox and agent document that calls `bd` works by changing only which program is invoked, with the same flags, JSON fields, exit codes and environment variables.

**Why this priority**: About 30 code files and every agent prompt depend on it; breaking them costs more than the speed gains.

**Independent Test**: Run the existing test suites of learner and link-inbox and a replay of the command table in the compat surface, comparing exit codes and JSON field presence against `bd` 1.3.1.

**Acceptance Scenarios**:

1. **Given** each command in the compat surface, **When** run against beads-lite, **Then** exit code and the JSON fields listed there match `bd` 1.3.1.
2. **Given** a command outside the surface, **When** run, **Then** it fails with a clear "not supported in beads-lite" message and a non-zero exit, never silent wrong output.

---

### User Story 4 - Move each project over with nothing lost, and be able to leave (Priority: P1)

The owner approves a switch per project. The migration copies every bead into the new store, proves nothing changed, archives the old store, and can export back to bd format at any time.

**Why this priority**: The tracker is the company's memory; no switch happens without proof.

**Independent Test**: Migrate a copy of whisper-flow, then company; compare per-bead counts before and after; export and re-import into stock `bd` on a scratch copy.

**Acceptance Scenarios**:

1. **Given** a project copy, **When** migrated, **Then** the verification report shows zero differences in beads, comments (author, time, text), dependencies (with type), labels and metadata keys (including `card_*` and `title_ar`).
2. **Given** a migrated project, **When** exported, **Then** stock `bd import` accepts the output and the re-export matches.
3. **Given** migration fails or differs, **When** reported, **Then** the live tracker is untouched.

---

### User Story 5 - Compact, filterable output (Priority: P2)

A caller can ask for exactly what it needs: a status, an assignee, a label, a parent, a limit, chosen fields. Defaults are short.

**Why this priority**: Fixes complaint 3 (too much unfiltered data); below speed because some filters already exist.

**Independent Test**: Compare default output sizes against SC-006 and check filters return correct subsets.

**Acceptance Scenarios**:

1. **Given** the default `list`, **When** run, **Then** one line per bead and at most the SC-006 line count, with a note when more exist.
2. **Given** `--json` with a field selection, **When** run, **Then** only those fields return.
3. **Given** a request for everything (explicit flag), **When** run, **Then** the full data returns, as bd does today.

---

### User Story 6 - Backups and history that do not slow work (Priority: P2)

The hourly backup and the "what changed, who, when" history work without blocking readers or writers, and without a store that grows without bound.

**Why this priority**: Today's hourly backup holds the lock for 7-109 s; history growth is a root cause.

**Independent Test**: Run backup while 10 agents work; confirm no call exceeds SC-004 and the store size after 30 days of simulated writes stays within SC-008.

**Acceptance Scenarios**:

1. **Given** a backup running, **When** agents read and write, **Then** latency limits still hold.
2. **Given** any bead, **When** history is requested, **Then** who changed what and when is shown.

### Edge Cases

- Two agents claim the same bead at the same moment: exactly one wins; the other gets the same refusal bd gives.
- A bead with 30+ KB comments, empty fields, Arabic text, or very long titles round-trips intact.
- Power loss or process kill mid-write: the store opens next time with no corruption and no half-written bead.
- Board opened while the tracker is busy, or on a project with zero beads.
- Data created by a newer or older beads-lite than the running tool: refused with a clear message, not misread.
- Custom statuses (owner_decision, waiting_for_test, waiting_for_merge) and unknown future statuses still list and display.
- `--actor` and `BD_ACTOR`/`BEADS_ACTOR` precedence stays as in bd.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The tool MUST provide the command surface in "Compatibility surface" below with the same flags, JSON field names, exit codes, and environment variables as `bd` 1.3.1.
- **FR-002**: Reads MUST never wait on other readers, on the board, or on a running backup; a write MUST never be lost when it overlaps another write.
- **FR-003**: Default output of `show`, `list`, `comments`, `ready`, `blocked` MUST be bounded and compact (SC-006); full output MUST be available by an explicit flag.
- **FR-004**: `list`/`ready`/`blocked` MUST support filtering by status, assignee, label, parent, type, priority and free text on title, and selecting JSON fields.
- **FR-005**: `ready` and `blocked` MUST follow bd's meaning (open work with no unresolved blocking dependency, and its complement).
- **FR-006**: Custom statuses MUST be supported without code change to the tool.
- **FR-007**: Every change MUST record who, what, and when, retrievable per bead, with bounded storage growth.
- **FR-008**: `export` MUST write bd-compatible JSONL of the full store; `import` MUST accept bd JSONL. Round trip MUST be lossless.
- **FR-009**: A migration command MUST copy a bd project into the new store, produce a per-bead verification report, archive (not delete) the old store, and leave the old store untouched on any failure.
- **FR-010**: The board MUST keep the address `127.0.0.1:3000`, the link format `#/issues?issue=<id>`, and the columns of today's patched board, MUST update on change without one tracker query per column, and MUST support the actions the owner uses today (view, change status/assignee, comment).
- **FR-011**: The board MUST bound each column (recent slice plus "show more") so large closed counts do not slow it.
- **FR-012**: No server listens on any interface except loopback; no secrets are stored in data or logs.
- **FR-013**: The tool MUST run on Windows 11 with no additional installs, no visible console windows when started by tools or schedulers, and no always-on process other than the board when it is open.
- **FR-014**: Unsupported bd commands MUST fail loudly with a non-zero exit and a pointer to the supported list.
- **FR-015**: Each project keeps its own store; one project's load MUST NOT slow another's.

### Compatibility surface (from bd-surface.md; ~/company scan only, E:/Projects scan still to do)

| # | Command / flags |
|---|---|
| 1 | `show <id> --json` (incl. `--include-dependents` used by the board) |
| 2 | `update --claim, -s, -a, --design, --set-metadata, --if-status, --if-assignee` |
| 3 | `comments --json`; `comments add -f / -a` |
| 4 | `list --status --assignee --json`; `--limit 0`; `--tree=false` |
| 5 | `create --silent -a -p --parent --body-file` |
| 6 | `close --reason --force` |
| 7 | `ready` (text and `--json --limit`) |
| 8 | `blocked`; `dep add/remove` |
| 9 | `label add` |
| 10 | `export -o` |
| 11 | `config get/set` |
| 12 | `init --non-interactive` |
| 13 | `count`, `import`, `bootstrap` (the `dolt push` backup step is replaced: see FR-009 and User Story 6) |
| 14 | `--actor` on every call |

JSON fields read by callers: title, status, metadata, created_by, comment author, comment text (plus every field the board shows). Environment: BD_ACTOR, BEADS_ACTOR, BD_BIN. Metadata keys that must survive: `card_*`, `title_ar`, `card_state`, `card_round`, `asked_at`.

### Key Entities

- **Bead**: id, title, description, design, status (including custom), priority, type, assignee, parent, created/updated/closed times, created_by.
- **Comment**: bead, author, time, text.
- **Dependency**: from-bead, to-bead, type (blocks, parent-child, etc.).
- **Label**: bead and name.
- **Metadata**: bead, key, value.
- **Change record**: bead, who, what changed, when.
- **Project store**: one per project, holding the above.

## Success Criteria *(mandatory)*

### Measurable Outcomes

"Whole command" means wall-clock from launch to exit, on the owner's PC, median of 20 runs with the board open and 3 agents active. "Today's data" = the real company project at 1,619 beads (the largest); "12x" = a scaled copy of about 19,400 beads with proportional comments, deps and labels.

- **SC-001**: `show`, `list` (default), `ready`, `comments`, `count` each finish in at most 0.5 s on today's data and at most 1.0 s on 12x data (baseline today: 2-10 s).
- **SC-002**: Board shows all columns usable within 2 s of opening on today's data and 4 s on 12x (baseline: 30-90 s to settle).
- **SC-003**: A change made by an agent is visible on the open board within 2 s on today's data and 3 s on 12x, and does not push any agent command above SC-001.
- **SC-004**: With 10 parallel agents doing mixed reads and writes plus a backup running, 0 failures, 0 lost writes, and no call over 2 s.
- **SC-005**: A write command (`create`, `update`, `comments add`, `close`) finishes in at most 0.5 s on both data sizes (baseline 1.2-2 s).
- **SC-006**: Default `show` output is at most 8 KB and default `list` at most 50 lines, even for the 246 KB bead; the full output is one flag away.
- **SC-007**: Migration of every existing project shows 0 differences in beads, comments, dependencies, labels and metadata keys, and the exported file re-imports into stock `bd` with 0 differences.
- **SC-008**: Store size on disk for company after 30 days of simulated writes at 3,000 writes/day stays under 100 MB (baseline: 562 MB after 4 days of history).
- **SC-009**: The existing learner and link-inbox test suites and the command replay in User Story 3 pass with no change other than the program invoked.
- **SC-010**: Cost to run: 0 paid services, 0 new installs, 0 processes running when the board is closed and no command is active.

## Assumptions

- Users are the owner and Codesters' agents on one Windows 11 PC; no remote or multi-machine use in v1.
- SQLite is the store (owner's decision); the CTO plan chooses how the CLI and board reach it.
- The tool is a fork: upstream beads features Codesters does not use (federation, molecules, Dolt branches and time travel) are out of scope.
- The command surface is from the ~/company scan only; commands used by E:/Projects repos (rayeh, fatura, whisper-flow, abduu-tasks) must be scanned before the plan is final.
- Backup stays hourly and off-machine, but its mechanism may change from `bd dolt push`/`bd export`; the backup runbook is updated with it.
- Timings are for the owner's PC; a slower PC is not a target.
- Defaults are chosen where the owner has not spoken; see Open Questions.

## Open Questions for the Owner

1. Is per-bead change history (who/what/when) enough, or do you need time travel to restore the whole tracker to an earlier moment? (Assumed: per-bead history plus hourly backups.)
2. Switch order: assumed one project at a time, whisper-flow first, company last, each needing your OK. Right?
3. Must the board look exactly like beads-ui today, or may the layout change to be faster? (Assumed: same look, same links.)

## Out of Scope

- Changing the issue model, adding new fields, or multi-user sync.
- Federation, molecules, wisps and other upstream features.
- Any paid service, hosted server, or account.
