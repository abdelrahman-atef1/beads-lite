# Contract: project switch (US8, FR-017)

Tool: `bd lite-switch <announce|ack|status|freeze|migrate|flip|rollback>` (fork), run by SRE per project; state in `.beads/lite-switch.json`:

```json
{ "state": "announced|frozen", "project": "rayeh", "announced_at": "...", "freeze_at": "...",
  "sessions": [ { "who": "...", "seen": "...", "ack_at": null } ], "by": "sre" }
```

| Step | Who | Effect | Exit / message |
|---|---|---|---|
| announce `--at HH:MM` | SRE | writes state `announced`; lists active sessions (Claude Code transcripts under the project folders and worktrees changed in the last 2 h); CoS posts the notice on the company bead | every fork command in the project prints one stderr notice line; writes still allowed |
| ack | any session | records `ack_at` for its actor | 0 |
| freeze | SRE | only when every session acked or has made no write for the window (default 15 min); state `frozen`; holds upstream `.beads/embeddeddolt/.lock` | fork writes: exit 3, "<project> is frozen for the beads-lite switch (since HH:MM, by sre); retry after it ends"; reads work; upstream bd fails on the lock |
| migrate | SRE | W8 copier + verifier into a new `beads.db` | report path; non-zero on any difference |
| flip | SRE | `metadata.json` backend `sqlite`; state file removed; lock released; callers restarted (board, Telegram bridge) | 0 |
| rollback | SRE | backend `dolt`; new `beads.db` moved to the archive; state removed | 0; the project is exactly as before the freeze |

Guarantees: nothing is copied before `freeze`; no write is accepted by either store between `freeze` and `flip`/`rollback`; `embeddeddolt/` is never modified or deleted (archived copy kept 30 days).
