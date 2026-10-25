# Data model: 001-fast-compact-tracker

The model is upstream bd v1.3.1's, unchanged (spec: "no new fields"). Column-level schema, types and indexes: [ADR 0001](../../docs/decisions/0001-frozen-sqlite-schema.md). This file maps the spec's entities to tables and lists the rules the code must keep.

| Spec entity | Table(s) | Rules to keep |
|---|---|---|
| Bead | `issues` (+ `wisps`, empty in practice) | id unique, prefix from config; `status` free text (custom statuses from `custom_statuses`, FR-006); `title` <= 500 chars (validated in Go); `is_blocked` is denormalised and must be recomputed by issueops on every dependency or status change exactly as upstream; `updated_at` set by issueops, never by the schema. |
| Comment | `comments` | author, `created_at` (second precision), text byte-equal through migration; keyset order `(issue_id, created_at, id)`. |
| Dependency | `dependencies` | `type` kept (`blocks`, `parent-child`, ...); exactly one target (issue, wisp or external); FK cascade on delete. |
| Label | `labels` | `(issue_id, label)` unique. |
| Metadata | `issues.metadata` (JSON text) | keys `card_*`, `title_ar`, `card_state`, `card_round`, `asked_at` survive migration and round trip; semantic JSON equality is the bar. |
| Change record | `events` (audit, written in the mutation transaction) and `lite_issue_versions` + `lite_version_text` (per-write snapshots for `bd history`) | one version per write transaction per issue touched; versions exist iff the write committed; `committer` = actor (`--actor` > `BD_ACTOR` > `BEADS_ACTOR` > upstream default). |
| Project store | `.beads/beads.db` (+ `-wal`, `-shm`), `metadata.json` backend `sqlite` | `PRAGMA user_version = 1`; newer version refused; never on a network or synced folder. |
| Switch state (new, transient) | `.beads/lite-switch.json` | states `announced` -> `frozen` -> removed (done or rolled back); see [contracts/switch.md](./contracts/switch.md). |

Other v1.3.1 tables are created and kept as upstream uses them (`config`, `metadata`, `local_metadata`, `issue_counter`, `child_counters`, `custom_statuses`, `custom_types`, `leases`, `repo_mtimes`, `provenance_events`, `bd_events_journal`, `bd_events_seq`, `schema_migrations`).

State transitions: upstream's (open, in_progress, blocked, closed, deferred, plus custom). The board's columns are statuses, not new states.
