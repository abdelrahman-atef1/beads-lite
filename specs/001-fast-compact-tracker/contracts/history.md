# Contract: `bd history`

Caller that depends on it: rayeh `tools/deploy/gate-lib.mjs:122-145` (`askedSinceFromHistory`, `loadHistory`), also `field-brief.mjs:135`, `send-gates.mjs:502`.

## `bd history <id> --json [--limit N]`

stdout: JSON array, newest first (upstream order), each element:

```json
{ "CommitHash": "lite-1234", "Committer": "swe", "CommitDate": "2026-10-25T09:30:00Z",
  "Issue": { "id": "rayeh-abc", "status": "owner_decision", "assignee": "abdelrahman", "...": "every Issue field as in show --json" } }
```

- One element per write command that changed the bead (create, update, close, claim, metadata, ...), plus the migrated Dolt versions from before the switch.
- `CommitDate` is UTC, second precision. `Issue` carries every field upstream's `types.Issue` has; large text fields come from the deduplicated bundle (ADR 0001 section 4).
- Unknown id: `[]` and exit 0 (upstream GH#3502 behaviour). Never an error for "unsupported".
- Acceptance: for each rayeh bead in the golden corpus, `askedSinceFromHistory(fork output)` equals `askedSinceFromHistory(upstream output)`; after migration, the same holds against the pre-migration upstream output.

## `bd history <id> --events [--json] [--limit N]`

Audit events from the `events` table, same shape as upstream (`types.Event`).
