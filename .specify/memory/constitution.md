# beads-lite Constitution

beads-lite is Codesters' private fork of the beads task tracker (bd) and its board (beads-ui),
reshaped so that speed and compactness do not degrade as the number of tasks grows.

## Core Principles

### I. Zero Cost, Zero Idle Cost
The tool MUST run with no paid service, no account, no API key and no always-on background
process that is not needed for the current use. Nothing runs, and nothing is spent, while nobody
is using it. Rationale: Codesters' mission is free tools kept sustainable by near-zero cost.

### II. Local-First, Own Your Data
All data lives in files on the owner's machine. The tool MUST work with the network off. Any
off-machine copy is a backup the owner controls, never a dependency. Data not needed is not
stored. Rationale: no outage, quota or vendor change can stop the owner's work.

### III. Speed Independent of Task Count (NON-NEGOTIABLE)
Interactive commands and the board MUST stay fast as the task count grows: growth in beads,
comments or history MUST NOT visibly slow a command. Speed targets in the spec are release
gates, measured on real data and on 12 times that data. Rationale: this is the reason the
fork exists; "count must not matter" is the owner's requirement.

### IV. Compact by Default
Output is filtered and bounded by default; the full dump is always available on explicit
request. A command never prints data the caller did not ask for. Rationale: agents pay in time
and tokens for every unneeded line.

### V. Compatible with Existing bd Usage
The commands, flags, exit codes, environment variables and JSON fields that Codesters scripts,
hooks, agents and documents use today MUST keep working unchanged, so callers change at most
the program they invoke. A removed or altered behaviour inside that surface is a defect.
Rationale: about 30 scripts and every agent prompt call bd.

### VI. No Data Loss, Reversible Switch
Migration MUST preserve every bead, comment (with author and time), dependency (with type),
label and metadata key, proven by a count-and-diff check showing zero differences. The old
store is archived, never deleted. Export in bd-compatible form MUST always work so the owner
can leave within a day. Nothing replaces a live tracker without the owner's approval.
Rationale: the tracker is the company's memory.

### VII. Boring Technology, Windows First
Prefer unfashionable, widely used, built-in components that need no install or native build.
The tool MUST work on Windows 11 with no visible console windows, and MUST NOT depend on
anything we cannot run for years unattended. Rationale: we maintain this ourselves, forever.

### VIII. Safe by Default
Servers listen on loopback only; no secrets in data or logs; concurrent agents never corrupt
data or block each other for more than a moment. When a limit is reached the tool gets more
limited, never broken. Rationale: many agents share one machine.

## Scope and Constraints

- WHAT before HOW: specs state outcomes; the CTO owns the plan and technology choice.
- Upstream beads features Codesters does not use (federation, molecules, Dolt branches and
  time travel) are out of scope unless a spec adds them.
- Project tracking for this fork uses GitHub Spec Kit, not beads (owner, 2026-10-25).

## Development Workflow

- Every change traces to a requirement in a spec under `specs/`.
- Performance and compatibility claims are verified by measurement, not assumption; a gate that
  was skipped is reported as "not exercised", not as passed.
- Branch `lite` is the working branch; `main` mirrors upstream.

## Governance

This constitution supersedes other practice for this project. An amendment needs a written
reason, the owner's approval, and a version bump (MAJOR: principle removed or redefined;
MINOR: principle or section added; PATCH: wording). Every spec, plan and review MUST state
compliance with Principles I-VIII, and any deviation must be justified in writing.

**Version**: 1.0.0 | **Ratified**: 2026-10-25 | **Last Amended**: 2026-10-25
