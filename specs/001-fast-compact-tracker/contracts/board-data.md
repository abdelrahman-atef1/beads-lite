# Contract: board <-> tracker

Board server (repo `beads-lite-ui`, `server/`) talks to the tracker only by running `bd` (fork or upstream; same arguments), never by opening the store file.

| Moment | Calls | Rule |
|---|---|---|
| Open / any refresh | 1x `bd list --json --tree=false --limit 0` (all non-closed) + 1x `bd list --json --tree=false --status closed --limit <N>` + `bd ready --json --limit 1000` + `bd blocked --json` | results shared by every column subscription; split by status in the server (columns: ready, in_progress, owner_decision, waiting_for_test, waiting_for_merge, blocked, closed, plus any custom status) |
| Detail | `bd show <id> --json --include-dependents` | cached until the next change event |
| Change detected | watcher on `.beads/` (Dolt dir or `beads.db-wal`), debounce 250 ms | at most one refresh in flight; a change during a refresh schedules exactly one more |
| Mutation | `bd update` / `comments add` / `close`, then one refresh | no extra `show` |
| Concurrency | at most 4 bd processes at once, interactive before background | all spawns `windowsHide: true`, timeout and process-tree kill kept |

Listens on 127.0.0.1:3000 only. Link `#/issues?issue=<id>` opens that bead (US2.3, US7.4). Closed column shows the newest N with "show more" (FR-011).
