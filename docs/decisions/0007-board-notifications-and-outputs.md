# ADR 0007: Board notifications to the phone (per-project switch) and the Outputs panel

- Status: proposed (CTO design, 2026-10-28). Web Push build waits for the SEC verdict; the local-only parts do not.
- Bead: company-1bo5.6.13 (parent company-1bo5.6). Builds on ADR 0006 (board doors, cards, ask thread, wake).
- Owner inputs, 2026-10-28, verbatim on company-1bo5.6: «وعايز يظهر برضو اشعارات لاي ديسجن محتاجني في اي مشروع (يتفعل ويتقف بير بروجيكت) / والاشعار يكون برضو لرده على سؤال على ديسجن» and «في في كلود خانة اسمها outputs هي بتبقا على مستوى المحادثة عايزها ظاهرة في البورد برضو».
- Author: شلبي (CTO).

## 1. Prior art

Queries, 2026-10-28, `gh search repos --sort stars`: "web-push", "web-push vapid node", "web push notifications", "ntfy", "ntfy push notifications", "gotify", "gotify server", "UnifiedPush", "self-hosted push notification android", "pwa push notification tailscale", "beads ui", "beads notifications", "claude code outputs artifacts panel", "agent run artifacts tracker", "commit links issue tracker"; `npm view web-push`. Board prior art (beads-ui, beads_viewer, perles, Plane, Linear, Jira) is in the PM plan on company-1bo5.6; wake prior art in ADR 0006.

| Found | Stars / activity / licence | What it does | Verdict |
|---|---|---|---|
| Web Push standard (RFC 8030, 8291 encryption, 8292 VAPID) + web-push-libs/web-push (npm `web-push` 3.6.7) | 3.6k, repo 2026-10-26, npm last publish 2024-01, MPL-2.0 | Server sends an end-to-end encrypted message to the browser's push service; the service worker shows it with the tab closed | **Adopted.** No account: VAPID keys are generated locally. Library used unmodified (MPL is file-level, fine) |
| binwiederhier/ntfy | 35.1k, 2026-10-27, Apache-2.0 | Push server + Android/iOS app | Rejected as primary: public ntfy.sh carries cleartext titles through a third party on a guessable topic; self-hosted needs the phone's Tailscale VPN up all day (battery) and a new app. **Exit path** if Web Push dies |
| gotify/server + gotify/android | 16.1k / 1.5k, 2026-10, MIT | Self-hosted push over a websocket | Rejected: Android only, a second server, same always-connected cost as self-hosted ntfy |
| UnifiedPush (mollysocket, uniqush, aerogear) | 0.4k-1.6k | Push distributor standard / servers | Rejected: built for native apps; Web Push already reaches the browser |
| Telegram bot | in use | | Retired for decisions by the owner (ADR 0006) |
| Email (SMTP), Pushover, Pushbullet | | | Rejected: an account or a stored credential; Pushover is paid |
| mantoni/beads-ui (our upstream), Mosnar/beadazzle | 763 / 16, MIT | Beads boards | No notifications, no outputs. We add both |
| GitHub issue "Development" panel, Jira development panel (smart commits), Linear git linking | hosted | Commits/PRs linked to an issue by the issue key in the commit message | **Idea adopted** for commits: we already put the bead id in every commit subject |
| Claude "Outputs" pane | hosted | Files and artifacts of one conversation | The model the owner named; our "conversation" is the bead |
| "beads notifications", "claude code outputs artifacts panel", "agent run artifacts tracker", "pwa push notification tailscale", "self-hosted push notification android", "commit links issue tracker" | nothing relevant | | Nothing to adopt |

## 2. Notifications

### N1. What notifies (two triggers only)
Both are detected by the board from the beads it reads; agents do nothing new.
- `decision`: a bead in a project whose switch is on enters `owner_decision`, assignee `abdelrahman`, and passes the card core's check (ADR 0006 D1, `card check`). No valid card, no notification (rule: no card, not in the queue). Key: `(project, bead, card hash)`, so a re-parked card with a changed question notifies again, an unchanged one does not.
- `answer`: a parked card whose `card_state` becomes `explained` (the asker answered his question, ADR 0006 D3). Key: `(project, bead, round)`.
- Not notified: new tasks, comments, status changes, anything by `owner-board` (his own actions).

### N2. How the board sees changes in every project
The board already watches the open workspace's database. A `notifier` module in the board server adds one `fs.watch` per registered project with the switch on (the project's `.beads` folder, recursive), debounced 5 s; a change triggers one background-priority `bd list --status owner_decision --json` in that project, diffed against `notified.json`. A 30 min rescan covers a missed file event (same cap as the existing watcher). Idle: zero bd spawns between database writes apart from the rescan. Rejected: beads hooks (`.beads/hooks/on_update`): a program spawn on every agent `bd` call, installed per repo, on a machine whose antivirus already flags spawn retries. Rejected: agents emit a wake-style event when they park a card: relies on every agent remembering, and an agent that forgets is the case that matters.

### N3. How it reaches the phone with the tab closed: Web Push
- The phone door (ADR 0006 section 2, `https://<pc>.<tailnet>.ts.net`, a secure context) serves a web app manifest and a service worker, so the board installs as an app (Android Chrome: any time; iPhone: iOS 16.4+, Add to Home Screen first). Settings > Notifications > "Turn on for this phone" calls `PushManager.subscribe` with the board's VAPID public key.
- VAPID key pair: generated on first start into `%USERPROFILE%\.codesters\board\vapid.json` (ACL owner + SYSTEM, outside every repo, never in a bead or a backup repo). Subscriptions (endpoint URLs are capabilities) in `push-subs.json` beside it. Lost key = re-subscribe once; nothing else depends on it.
- Delivery: the board posts the encrypted payload to the endpoint (Google FCM for Chrome, Apple for Safari, Mozilla for Firefox). The phone needs no Tailscale connection to receive; tapping opens the bead on the phone door, which does.
- Payload (encrypted end to end; the push service sees only size and timing), JSON with no free text: `kind`, project key, bead id, `card_title` (neutral by rule), `at`. Desk and labelled beads (R18): no title, only "A decision waits at the PC (<project>)". `answer`: "Answer to your question: <card_title>". No `card_q`, options, comments or his words.
- Caps (Abuse surface): one push per key; per project at most 1 push per 2 min, extras folded into one "N decisions waiting in <project>"; 50 pushes per day in total, then one "more on the board" and silence until midnight. A 404/410 from the endpoint deletes that subscription.
- PC door: the same subscribe works in desktop Edge/Chrome on `127.0.0.1` (also a secure context), optional.

### N4. Per-project switch
In the board's local config `%USERPROFILE%\.codesters\board\notify.json`: `{ "projects": { "<key>": { "on": true } }, "mute_until": null }`. Default on for every registered project, including new ones (he asked for any decision in any project). Set from the board: a bell toggle on each project in the switcher and a list in Settings, both doors, as a socket mutation behind the ADR 0006 D8 Origin check and token. Rejected: in each project's `.beads/config.yaml` (committed, travels to other machines and clones, a preference is not project data); in bead metadata (would need a bead per project and mixes his preference into the task log). A switched-off project's watcher is closed (no cost).

### N5. In the board, always (Graceful degradation)
A bell in the header with the same events (newest first, unread count, "mark read"), kept in `notified.json` for 30 days. If push is off, unsupported, out of cap, or the push service is down, he sees everything on the board; push is only the alarm. Board down (it starts at logon, task CompanyBeadsUI): on the next start the notifier diffs against `notified.json` and sends one digest push, not one per bead.

## 3. Outputs

Claude's Outputs belong to a conversation; ours belong to a bead, and roll up: a story shows its own and its children's outputs, an epic its stories', and each project has an Outputs view (filter by kind, newest first). Four sources, so most outputs appear without any agent effort:

| Source | How | Effort for agents |
|---|---|---|
| Commits | `git log --all` in the project's repo, bead ids taken from the subject (`(company-1bo5.6.13)` convention, several ids allowed), cached per repo and read incrementally; shows short hash, subject, branch, "landed" if on the default branch, a link to the private remote | none (existing convention) |
| Recorded outputs | Comments whose first line is exactly `OUTPUT v1 <kind>`, second line one JSON object `{ref,label,commit?}`, written by `output add` (below) | one command per deploy/artifact/APK/doc |
| Mentioned links | `https://` URLs in any comment of the bead (claude.ai artifacts, PRs, deploy hosts), deduplicated against recorded ones, shown as "mentioned" | none |
| Attachments | `metadata.attachments` from bug reports (ADR 0006 D7), shown from `/att/` | none |

### O1. Why comments, not bead fields
Comments are append-only: two agents adding outputs at once lose nothing. A metadata array (`--set-metadata outputs=[...]`) is read-modify-write and drops entries under the parallel runs we have every day. Comments also keep who and when.

### O2. `output add` (company tool)
`~/company/scripts/outputs/output.mjs`, Node built-ins only: `output add <bead> --kind deploy|artifact|build|doc|pr|link|file --ref <url-or-path> --label "<=80 chars>" [--commit <sha>]` and `output list <bead>`. It validates and then writes the comment through `bd comments add` (actor from BD_ACTOR). Refs: `https://` URLs, or `http://127.0.0.1` / tailnet URLs; a path must be repo-relative inside the bead's repo or an attachments path; no absolute paths outside, no URL with a query or fragment that looks like a token, the label and ref pass the card scan's secret check (refuse, never echo). The board renders every output as text, links only for `https:`/allowed schemes, `rel="noopener noreferrer"`.

### O3. Making agents record them
The existing rule "each deploy is logged on its bead (what, commit, URL)" gets its format: `output add --kind deploy`. One line in AGENTS.md and the closing checklist in the role files: deploys, published artifacts, APKs, ADRs and docs are recorded with `output add`. AGENTS.md and role files are owner-approved text: CoS proposes it (child bead). Commits and links need nothing, so a forgetful agent still leaves most of its outputs visible.

### O4. Cost of reading outputs
Computed when a detail or Outputs panel opens, never polled: one `bd show --json` per bead in the rollup (capped at 50 beads, then "show more"), cached until the project's database changes; git scan incremental from the last seen commit per ref. Rejected: an outputs table in the beads database (schema change in the frozen schema, ADR 0001); a separate outputs index service (always-on, a second source of truth).

## 4. Cost
- Per active user per month: $0. Web Push services (FCM, Apple, Mozilla) are free with no account for VAPID senders; everything else runs in the board process on the PC.
- Per idle user: $0. Idle project = an open file watch; switched-off project = nothing; no push without an event. Model cost: zero turns (no agent involved).
- Per event: one HTTPS post of under 4 KB. Daily cap 50.
- First limit we hit: our own cap of 50 pushes/day (by design). Push services publish no hard quota for this volume; FCM throttles far above it.
- SEC review needed (data leaves the PC to a push service; capability URLs and a signing key stored locally): child bead, blocks the Web Push build only.

## 5. Exit path
If browser push changes terms or breaks on his phone: self-hosted ntfy on the PC behind the same notifier (one sender function swapped), phone on Tailscale; the bell keeps working meanwhile. If `web-push` is abandoned: RFC 8291/8292 in `node:crypto` (~150 lines). Outputs are plain comments and git history: any future board reads the same `OUTPUT v1` lines.

## 6. Success condition
With the board tab closed and the phone locked: an agent parks a valid card in a switched-on project and a push arrives within 30 s showing only project and card_title (desk: pointer only); tapping opens the bead on the phone door; an `explained` answer to his question pushes once; a switched-off project pushes nothing and its watcher is closed; an invalid card pushes nothing; the bell lists the same events with push off. A story's Outputs panel shows its children's commits (by id in the subject), `OUTPUT v1` records, mentioned links and report images, with no bd call when nothing changed.
