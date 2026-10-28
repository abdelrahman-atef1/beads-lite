# beads-lite

A fork of [gastownhall/beads](https://github.com/gastownhall/beads) (`bd`) that keeps the same commands and JSON but stores issues in **SQLite (WAL) instead of embedded Dolt**, so one `bd` call takes milliseconds instead of seconds and does not grow a history store with every write.

It exists for one reason: on our machine (about 1,600 beads in the biggest project, many agents at once) upstream `bd` took 2-10 s per command, queued behind other processes, and dumped hundreds of KB per `show`. Task count was not the cause; per-process Dolt start-up, one-at-a-time locking and an ever-growing history were. Numbers and the full case: [spec](https://github.com/abdelrahman-atef1/beads-lite/blob/lite-integration/specs/001-fast-compact-tracker/spec.md).

Companion repo: [beads-lite-ui](https://github.com/abdelrahman-atef1/beads-lite-ui), the board that runs on top of this `bd`.

## Branches (read this first)

| Branch | What it is |
|---|---|
| `main` | Upstream `gastownhall/beads` exactly as cloned (`e866614a6`), plus this README. Kept so the diff against upstream stays visible. |
| `lite-integration` | **Our working branch.** Upstream tag **v1.3.1** plus our changes (SQLite store, `migrate-to-sqlite`, `lite-switch`, passthrough, specs, ADRs). Build from this one. |
| `lite` | Early base (v1.3.1 + Spec Kit files only). Superseded by `lite-integration`. |
| `swe/*`, `sre/*`, `cto/*`, `qa/*` | Task branches; merged into `lite-integration` by the CTO. |

We only ever merge an upstream **tag**, by a recorded decision ([ADR 0002](https://github.com/abdelrahman-atef1/beads-lite/blob/cto/extract/docs/decisions/0002-upstream-merge-policy.md)). The full upstream README is kept as [`README.upstream.md`](README.upstream.md) on `lite-integration`.

## What is different from upstream

- **Store:** one `beads.db` per project (`.beads/`), SQLite in WAL mode, pure Go (`CGO_ENABLED=0`). Schema is frozen: [ADR 0001](https://github.com/abdelrahman-atef1/beads-lite/blob/lite-integration/docs/decisions/0001-frozen-sqlite-schema.md).
- **Same surface:** the commands agents use (`list`, `show`, `ready`, `create`, `update`, `close`, `comments`, `dep`, `export`, ...) and their JSON are held to upstream 1.3.1 output by a parity corpus. See [`contracts/`](https://github.com/abdelrahman-atef1/beads-lite/tree/lite-integration/specs/001-fast-compact-tracker/contracts).
- **Compact output:** `show`/`list` print only what was asked for, not the whole history.
- **Passthrough:** a project that is still on Dolt keeps working; the fork hands those reads and writes to an upstream 1.3.1 binary. Projects switch one at a time.
- **Local commands:** `migrate-to-sqlite` (Dolt to SQLite, verified bead by bead) and `lite-switch` (`announce`, `ack`, `freeze`, `flip`, `rollback`).
- **No usage metrics.**

Everything else is upstream code, unchanged. Upstream is MIT (Copyright (c) 2025 Beads Contributors); [`LICENSE`](LICENSE) and `THIRD_PARTY_LICENSES` are kept as they came. We add no separate licence for our changes.

## Install and run on the owner's PC

All callers (the `bd` on PATH, the learner, link-inbox, the Telegram bridge, the board, the backup scripts) end at one file: `%APPDATA%/npm/node_modules/@beads/bd/bin/bd.exe`. We replace that file (the "shim") so every caller gets the fork at once.

```
# run from a lite-integration checkout. Build (Go 1.26.5, no cgo)
CGO_ENABLED=0 GOTOOLCHAIN=local go build -o <out>/bd-lite.exe ./cmd/bd

# install the shim (records the sha256 of upstream, keeps it as bd-upstream.exe)
bash docs/runbooks/cutover/shim.sh install <abs path>/bd-lite.exe

# switch one project (frozen -> migrate -> flip -> smoke; rolls back by itself on any failure)
bash docs/runbooks/cutover/project.sh <name> <abs project dir>
```

**Roll back:**

```
bash docs/runbooks/cutover/shim.sh rollback                              # upstream bd.exe back, byte for byte
bash docs/runbooks/cutover/project.sh <name> <abs project dir> rollback  # one project back to Dolt
```

Beads written on SQLite after a switch are not in Dolt: `bd export -o <file>.jsonl` before a project rollback. `embeddeddolt/` is never deleted.

Full steps, order of projects, rehearsal evidence and known defects: [runbook `cutover.md`](https://github.com/abdelrahman-atef1/beads-lite/blob/lite-integration/docs/runbooks/cutover.md). Rules: every path absolute; one project at a time; stop on a refused program start.

## The board

The board is [beads-lite-ui](https://github.com/abdelrahman-atef1/beads-lite-ui), started at logon by the company script `scripts/start-beads-ui.sh` (in the private company repo). It listens on `127.0.0.1:3000`; the phone door is published tailnet-only with Tailscale serve (never Funnel) so the phone reaches it over HTTPS (ADR 0006, section 2). `BDUI_ROLLBACK=1` starts the old npm board instead. The board uses this `bd` through `BD_BIN`. Board design for the phone, decision cards and notifications: [ADR 0006](https://github.com/abdelrahman-atef1/beads-lite/blob/lite-integration/docs/decisions/0006-board-decision-cards-and-wake.md), [ADR 0007](https://github.com/abdelrahman-atef1/beads-lite/blob/lite-integration/docs/decisions/0007-board-notifications-and-outputs.md).

## Docs

- Spec Kit: [`specs/001-fast-compact-tracker`](https://github.com/abdelrahman-atef1/beads-lite/tree/lite-integration/specs/001-fast-compact-tracker) (spec, plan, research, contracts, tasks).
- ADRs: [`docs/decisions`](https://github.com/abdelrahman-atef1/beads-lite/tree/lite-integration/docs/decisions): 0001 frozen schema, 0005 prior art, 0006 and 0007 board. 0002 (upstream merge policy), 0003 (migration extract), 0004 (G1 review) are on branches `cto/extract` and `cto/g1` until merged.
- Runbook: [`docs/runbooks/cutover.md`](https://github.com/abdelrahman-atef1/beads-lite/blob/lite-integration/docs/runbooks/cutover.md).
- Setup log: [`SETUP-NOTES.md`](https://github.com/abdelrahman-atef1/beads-lite/blob/lite-integration/SETUP-NOTES.md).

## Relation to other projects

- **Upstream** [gastownhall/beads](https://github.com/gastownhall/beads): where the code comes from. We track tagged releases only and send nothing back yet.
- **[dcosson/beads-lite](https://github.com/dcosson/beads-lite)** (MIT, one JSON file per issue): a different project with the same name. We evaluated it and did not adopt it (does not build on Windows, 64% exit-code parity on our corpus, targets bd 0.49): [ADR 0005](https://github.com/abdelrahman-atef1/beads-lite/blob/lite-integration/docs/decisions/0005-prior-art-dcosson-beads-lite.md). The name is shared by four repos; renaming ours is tracked in the company beads (company-1bo5.2).
- **[philcunliffe/beads-lite](https://github.com/philcunliffe/beads-lite)**: same idea (SQLite-only fork) on upstream 1.0.4, idle since May 2026. Read, not adopted.

Not a product and not supported for others: it is the tracker of one small company, published so the fork is auditable.
