# Contract: CLI compatibility surface

Oracle: upstream bd 1.3.1 on copies of the five real stores; outputs saved by QA in `parity/` (P0.2). A row passes when, for the same input store and arguments, beads-lite gives the same exit code, the same stdout JSON after normalisation, and the same side effects (read back with `show --json`).

Normalisation (and nothing else): timestamps to the second; ids minted during the run; key order inside JSON objects; `CommitHash` values in `history`.

| Row | Command | Contract |
|---|---|---|
| 1 | `show <id> --json [--include-dependents]` | same fields and values |
| 2 | `update --claim -s/--status -a --design --set-metadata --if-status --if-assignee` | same success/refusal and exit codes; claim race: exactly one winner |
| 3 | `comments <id> --json`; `comments add -f/-a` | author, time, text equal |
| 4, 17 | `list` with `--status --assignee -l/--label --all --json --limit 0 --tree=false` | same set and order |
| 5 | `create --silent -a -p --parent --body-file` | `--silent` prints only the id |
| 6 | `close --reason --force` | same |
| 7, 19 | `ready` (text, `--json --limit`, `--explain`), child visibility | same set |
| 8 | `blocked`; `dep add/remove` | same set; `is_blocked` equal on every bead after each step |
| 9 | `label add` | same |
| 10, 16 | `export -o`, `export --events` | upstream JSONL; stock bd 1.3.1 `import` accepts it |
| 11 | `config get/set` | same |
| 12, 21 | `init --non-interactive`, name from directory; `bootstrap --yes` | creates `beads.db`, backend `sqlite` |
| 13 | `count`, `import`, `bootstrap` | same |
| 14 | `--actor`, `BD_ACTOR`, `BEADS_ACTOR` precedence | same |
| 15 | `history <id> --json`, `--events` | see [history.md](./history.md) |
| 18 | `search <text>` | same set |
| 20 | `update --status <s>` | same as `-s` |
| 22 | `dolt push/pull`, `doctor --fix` | print the replacement (`bd backup snapshot`, `bd doctor`) and exit non-zero |

Outside the surface: a command that needs Dolt (`vc`, `branch`, `diff`, `federation`, ...) fails with "not supported in beads-lite", a pointer to this list, and a non-zero exit (FR-014). Never silent wrong output.

Text output (not JSON): `show` <= 8 KB and `list` <= 50 lines by default with a "more" hint; `--full` gives upstream text (research R7). `--fields a,b` on any `--json` command returns only those keys (opt-in, new).
