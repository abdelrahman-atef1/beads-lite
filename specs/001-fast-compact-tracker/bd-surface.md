# bd compatibility surface (Explore, 2026-10-25; scope ~/company only, E:/Projects not yet scanned)

~91 files match; ~30 code files run bd; ~20 distinct subcommands.

| # | Command / flags | Example |
|---|---|---|
| 1 | show --json | learner/lib/beads.mjs:103, link-inbox/lib/bd.mjs:30 |
| 2 | update --claim, -s, -a, --design, --set-metadata, --if-status, --if-assignee | learner/lib/beads.mjs:129 |
| 3 | comments --json; comments add -f / -a | commit-bead-hook.js:143, beads.mjs:121 |
| 4 | list --status --assignee --json | beads.mjs:95, bdui-patch-board.mjs:41 |
| 5 | create --silent -a -p --parent --body-file | link-inbox/lib/bd.mjs:40 |
| 6 | close --reason --force | link-inbox/lib/bd.mjs:34 |
| 7 | ready (prompts) | agents/swe.md:20 |
| 8 | blocked; dep add/remove | skills/task-planning/SKILL.md:66 |
| 9 | label add | skills/task-planning/SKILL.md:67 |
| 10 | export -o | backup-projects.sh:89 |
| 11 | config get/set | backup-projects.sh:96, new-project.sh:27 |
| 12 | init --non-interactive | new-project.sh:25 |
| 13 | count, import, bootstrap, dolt push | backup-restore.md:91 |
| 14 | --actor on every call | link-inbox/lib/bd.mjs:16 |
| 15 | list --status --limit 0 --tree=false | bdui-patch-board.mjs:41 |

JSON fields read: title, status, metadata, created_by, comment author, comment text.
Env: BD_ACTOR, BEADS_ACTOR, BDUI_BD_TIMEOUT_MS, BD_BIN, LEARNER_BD_JS, APPDATA.
Metadata keys: card_*, title_ar, card_state, card_round (asked_at in QA doc only).
