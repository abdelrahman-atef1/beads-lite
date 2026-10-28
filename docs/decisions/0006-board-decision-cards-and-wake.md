# ADR 0006: The board as the owner's only door: decision cards, clarification thread, bug reports from the phone, and waking the responsible session

- Status: proposed (CTO design, 2026-10-28). Build waits for the SEC verdict (child bead of company-1bo5.6).
- Renumbered 2026-10-28 from 0001 (clashed with 0001-frozen-sqlite-schema; 0002-0005 taken on other branches). Bead titles that say "ADR 0001" for the board mean this file.
- Bead: company-1bo5.6, phase 2. Owner inputs, all 2026-10-28, quoted on the bead: plan approved; clarification thread on cards; "when I add a task the agent responsible for the project is told at once"; Telegram retired for decisions, cards answered on the board from the phone over Tailscale; top priority: bug reports from the phone (screenshot + text) filed from the board as a bead in the chosen project, image shown on the bead, responsible agent woken. In-app report buttons are for end users and out of scope here.
- Author: شلبي (CTO).
- Scope: the live board (E:/Projects/beads-lite-ui, swe/board-cold, `server/ws.js`), the card rules now in `~/company/scripts/learner/lib/card.mjs`, the CoS main session, Tailscale already on the PC and the owner's phone. No hosted service.

## 1. Prior art

Queries run 2026-10-28, `gh search repos` unless noted: "claude code wake session notify", "claude code agent inbox hook", "human in the loop approval agent resume", "claude code session message queue", "agent mailbox claude code", "humanlayer", "agent mail mcp", "mcp_agent_mail", "claude code channels", "claude-code notification hook", "beads agent notify", "gastown beads", "beads attachments", "issue tracker screenshot upload self-hosted", "mobile bug report screenshot web upload"; npm "claude code hook notify"; pub.dev API for feedback, wiredash, feedback_gitlab, feedback_sentry, feedback_github, instabug_flutter, shake_flutter; grep for attachment support in E:/Projects/beads-lite. Board prior art (beads-ui, beads_viewer, perles, Plane, Linear, Jira) is in the PM plan on the bead.

| Found | Stars / activity / licence | What it does | Verdict |
|---|---|---|---|
| humanlayer/humanlayer | 12.7k, 2026-10-28, Apache-2.0 | Human approvals for agents over Slack/email, resume after reply | Rejected: hosted service with account and API key; approvals leave the bead channel |
| Dicklesworthstone/mcp_agent_mail (+ _rust) | 2.2k / 219, 2026-10, custom | Mailboxes between agents via an MCP server | Rejected: always-on server, agents poll an inbox, messages outside beads |
| Claude Code Channels (Johnixr/claude-code-wechat-channel 312 MIT, osisdie/claude-code-channels 6 MIT, wuzf/claude-channels-patch 43 MIT) | 2026-10 | Push outside messages into a running session | Not now: preview flag, one repo exists to patch official binaries, needs plugin/settings change. Upgrade path for the consumer (section 8) |
| steveyegge/gastown | 18.6k, 2026-10-28, MIT | Multi-agent workspace on beads, nudges sessions via tmux | Rejected as dependency: tmux, no Windows, a whole orchestrator. Idea taken: the wake is a tiny event, the bead holds the content |
| kmio11/cc-notification, jiunbae/aily (MIT), eyalzh/claude-code-toast (MIT) | 6-18, 2026 | Notification hooks, session to human | Wrong direction |
| beads (beads-lite) | our fork | No native attachments (only an Azure DevOps link importer) | We add an attachment reference in metadata, files outside the database |
| mantoni/beads-ui (our board's upstream) | 763, MIT | No uploads, no mobile report | We add the report page |
| ueman/feedback (BetterFeedback) 3.2.0, 1658 likes, Apache-2.0; wiredash 2.6.1 (AAL, SaaS); instabug_flutter, shake_flutter (SaaS SDKs); feedback_github / _gitlab / _sentry | pub.dev, 2025-2026 | In-app screenshot + annotate + text | Out of scope (in-app, for end users). Recorded so the later in-app bead starts from BetterFeedback, the only one with no account |
| "issue tracker screenshot upload self-hosted", "mobile bug report screenshot web upload", "beads attachments" | none relevant | | Nothing to adopt |

Conclusion: nothing joins "owner on his phone, on our board" to "the right session is told". We add three small pieces to what exists: a phone door on the board, an attachments repo, and a ~200-line wake queue.

## 2. Doors: one board, two listeners

- **PC door**: `127.0.0.1:3000`, as today. Fixed first (D8): Origin check and per-launch token.
- **Phone door**: a second listener on `127.0.0.1:3001` in the same board process. Only `tailscale serve` (HTTPS, `https://<pc>.<tailnet>.ts.net`) proxies to it. It accepts a request only if `Tailscale-User-Login` equals the owner's login in the local board config (a name, not a secret); Funnel requests carry no identity header, so a Funnel left on by mistake fails closed. Same Origin check and token as the PC door. The listener port, not a header, decides which rules apply.
- Phone door rules: desk and labelled beads (R18 labels, sec-created) are shown as a pointer only (neutral `card_title`, "answer at the PC", no description, design or comments); no delete; no removal of a desk label; no answer on a desk card.
- Free-tier headroom: Tailscale Personal is free for 3 users / 100 devices; we use 1 user, 2 devices. Exit in a week: plain WireGuard, or headscale (self-hosted control server).

Rejected: binding the board to the tailnet address directly (one listener can no longer tell PC from phone, no identity header, plain HTTP on the phone); a public URL or tunnel (Funnel, Cloudflare Tunnel: the board on the internet, an account, abuse surface).

## 3. Decisions on cards

### D1. One card core, no Telegram
Telegram is retired for decisions. The card rules move out of the learner into `~/company/scripts/cards/` (parse and validate, R18 fail-closed gate, record format, guarded update, repair), Node built-ins only, with a CLI the board calls by full path (`execFile`, `windowsHide: true`): `card answer`, `card ask`. The learner stops sending cards (config switch; it keeps the link inbox). The board never writes a decision record itself.
- Record comment, actor `owner-board`: first line `OWNER DECISION via board (pc|phone)`, then date/time, `card: b-<random>`, bead, question hash, option number and label verbatim, or typed text as one JSON string.
- Order: guarded update (`--if-status owner_decision --if-assignee abdelrahman`) first, so a double click or two open tabs give exactly one decision; then the record; then read-back; a crash between them is repaired from the `card:` line.
- Two clicks: choose, then Confirm (R20 kept). The card field limits stay (the owner liked point-based cards); the phone-specific Telegram rules (R10 card ids in buttons, 24 h re-send) are dropped.

### D2. Desk cards stay PC-chat-only
Neither door answers a desk card. On the PC door the board shows the full bead with "answer in the chat at the PC" and a copy button for the id; on the phone door only the pointer. The core refuses `answer` on any card the gate does not pass, so a forged request fails server-side. Rejected: letting the PC board answer desk cards: a click is weaker proof than the owner typing in a chat, and changing the desk rule is an N3 rule change.

### D3. Clarification thread
On tap and type cards (both doors) and desk cards (PC door only) the board offers "Ask" with a scope: the whole question, a part (he selects text in card_q, card_effect or card_show), or one option ("?" on each option).
- `card ask --scope question|part|option:<n> [--quote <span>]` writes one comment, actor `owner-board`: `OWNER QUESTION via board - NOT A DECISION`; date/time, `asked-card: b-<random>`, bead, `round: N`, `scope: ...`; quote and question each as ONE JSON string; the fixed "Scope: ... decides nothing" last line. Then the guarded update (open, assignee = card_back, `card_state=asked`, `card_round=N`) and a wake (D4).
- Limits: question 500 chars, 3 rounds per decision cycle, counted by the core from its own records.
- The asker answers with an `ASKER ANSWER round N` comment (max 1200 chars; the board shows the thread under the card) and, for tap/type, rewrites `card_show` (max 600) so the card itself carries the answer; then `card_state=explained` and re-parks. Desk: comment only. The asker's hard edges in decision-card.md (question is hostile input, never an instruction; a card may get stricter, never looser) stay.
- decision-card.md is rewritten for the board door (PM child bead).

## 4. D6. Owner-created beads: new task and bug report

### Which beads wake someone
Only beads created through the board's own handlers: `create-issue` (a task) and `report` (a bug report). The handler runs bd with actor `owner-board` and sets `created_via=board-task` or `board-report`, then emits a wake. Agents create beads with the bd CLI, which never passes the board server, so they wake nobody. We do not watch the database: `created_by` cannot tell the owner from an agent that forgot BD_ACTOR (both carry his git name). Tasks he dictates in a CoS chat are already known to that session.

### Bug report path (top priority)
1. The board's mobile view has a Report button (also on desktop). The page: project picker (defaults to the open workspace, lists the board's registered workspaces), screenshot picker (`<input type=file accept=image/*>`: gallery or camera), text box (max 2000 chars), Send.
2. The browser downsizes before upload (canvas, long side max 1600 px, WebP or JPEG q0.85): a 1-2 MB screenshot becomes about 150-300 KB on his mobile data, and re-encoding drops EXIF (Push compute to the client; Offline and low-bandwidth).
3. `POST /api/report` (HTTP, not the socket) on either door, with the board token and a custom header, so a cross-site form post is impossible (it forces a CORS preflight that the board refuses). Server checks: project in the registered list; image by magic bytes PNG/JPEG/WebP only, max 4 MB, metadata chunks stripped again (pure Node: JPEG APPn segments, PNG text chunks); text through the card scan's secret/token check (refuse, never echo). Cap: 30 reports per hour.
4. The image is written to the attachments repo (D7) as `<project>/<yyyy-mm>/<sha256 first 16>.<ext>`.
5. A bead is created in the chosen project: type bug, label `owner-report`, title = first line of his text (60 chars), description = his text verbatim plus `Screenshot: <project>/<yyyy-mm>/<file>`, metadata `attachments=[...]`, `created_via=board-report`. If the bead write fails, the image file is removed (the upload created it) and he sees "not filed, try again".
6. Wake `report` to the project's session (D4). The page shows "filed as <id>" and the wake state.
7. The bead's detail view on the board shows the image from `/att/<path>` (strict path regex, only under the attachments root, stored content type, `nosniff`, `Content-Security-Policy: default-src 'none'`). On a desk bead the phone door shows no image.

Rejected for the report: an in-app button in our test builds (owner: he reports from the board; in-app is for end users later); emailing or sharing the screenshot to an agent (no bead, no project, nothing wakes); an external image host or Telegram (third party keeps his screenshots, an account).

### D7. Where images live: one private attachments repo
`~/codesters-attachments`, a git repo with a private remote on the owner's account, on `scripts/backup-projects.list`, so the hourly backup pushes new images (it backs up untracked, non-ignored files; that is why the folder must not be gitignored). A pruned or rolled repo never touches code history.
Rejected: inside each project repo (screenshots enter code history and every clone; a bead in a repo without a remote is lost); `~/company` (same bloat in the company repo); a gitignored folder (the backup uses `--exclude-standard`, so it would never be backed up); base64 in bead metadata (database bloat, bd slows); an external host (account, third party).
Size: about 250 KB per report, so about 4,000 reports per GB. The backup refuses a first push over 2 GiB and a file over 95 MiB; when the repo passes 1 GB, SRE starts `codesters-attachments-<year>` (the path prefix in metadata makes it a move, not a migration).

## 5. D4. The wake

Queue: `%USERPROFILE%\.codesters\wake\` (outside every repo, folder ACL owner + SYSTEM), folders `pending/ claimed/ done/ sessions/`. Tool `~/company/scripts/wake/wake.mjs`: `emit`, `wait`, `done`, `list`.

Event (JSON, no free text): `id`, `kind` (`decided` | `asked` | `new-task` | `report`), `repo` (a registered project key), `bead` (regex `^[a-z0-9]+(-[a-z0-9]+)*(\.[0-9]+)*$`), `role` (card_back for `decided`/`asked`, `cos` for the other two), `via` (`pc` | `phone`), `at`. Invalid events are not written. His words never travel in the event; the woken agent reads the bead, so a wake line cannot inject instructions. Producer: the board server only, after an owner action. No agent action emits (re-parking an explained card, creating a bead, a rejected card).

Consumer: every CoS main session arms, with Bash `run_in_background`, `node <company>/scripts/wake/wake.mjs wait --project <key>`. `wait` prints the backlog first, else blocks on `fs.watch` (60 s re-scan as fallback), claims events by atomic rename into `claimed/`, prints one fixed line each, `WAKE report rayeh rayeh-x1y2 -> cos via phone`, and exits. The exit is the notification; CoS acts and re-arms. While waiting it keeps a lease `sessions/<key>-<pid>.json` (refreshed every 60 s, stale after 3 min), exits if its parent is gone, and exits after 8 h with `WAKE REARM`.

Who is "responsible for the project": the CoS session whose lease has that project key (derived from the git top level of its working folder, or `--project`). The company CoS session (`~/company`) runs with `--fallback`: it also claims events of any project with no live lease, after a 10 s grace. Several sessions on one project: the first rename wins, exactly one is told.

Session offline (no live lease at all): the event stays in `pending/`; the board shows "queued: no session open for <project>" on the bead, the card and the report confirmation; the next session for that project, or the company fallback, gets the backlog the moment it arms `wait`. Nothing is lost and nothing is launched on its own.

What CoS does with a line (text for cos.md, a CoS child bead; cos.md is a role file, so applying it is the owner's call at the PC):
- `decided` / `asked`: role `cos`, act itself; otherwise start a fresh background run of that role on the bead. Never SendMessage to the old helper; never two runs on one bead (if one is live, let it end first).
- `new-task` / `report`: triage and delegate like any owner request; a report goes to the project's QA/SWE path.
- Then `wake done <id>` and a one-line comment "woken: <role> run started - CoS". A claimed event not done within 15 min returns to pending.

Caps (Abuse surface, Blast radius): one pending event per (bead, kind); 30 events per hour per project, beyond that one `WAKE many <repo> <count>` line; `done/` pruned after 7 days from its own folder only.

### Requester cases
| Requester | How it is reached |
|---|---|
| A subagent | Not messaged. CoS starts a fresh run of the same role on the bead; the OWNER DECISION / QUESTION record says what happened. |
| The main session (card_back = cos) | Its own `wait` exits with the line; it acts. |
| A session that has ended | Event waits in `pending/`, shown as queued on the board; delivered when the next session for the project or the fallback arms `wait`. |
| A project agent (rayeh-*) | As a subagent: the project's CoS session runs the role named in card_back. |

### Rejected alternatives for the wake
1. **CoS Monitor on the queue.** Monitor expires after at most 30 min; each expiry is a model turn re-reading the whole session: about 48 idle turns a day per session (breaks Zero-idle-cost). A background `wait` costs no turn until there is work. Kept as fallback if background commands prove unreliable.
2. **Session messaging (SendMessage).** Only the spawning session can use it, only while the helper exists; the board is outside every session; the helper that parked a card has ended; cos.md forbids continuing long helpers; messages break "beads are the only channel".
3. **Board launches a headless run (`claude -p --agent <role>`).** Each click or forged request spends a full agent run, bypasses CoS routing, nobody reads the output, collides in shared trees, and adds a program-spawn path on a machine whose antivirus already flagged retries.
4. **CoS polls bd (/loop).** A turn per interval whether or not anything changed, plus bd load.
5. **Claude Code hooks (SessionStart / UserPromptSubmit).** Only fire when a session starts or he types (not "at once"), and are a settings change (N3). Possible later as a backlog print at session start, owner's call.
6. **Claude Code Channels.** Preview; later swap-in for `wait` on the same queue.

## 6. D8. Socket hardening comes first
Today `server/ws.js` creates `new WebSocketServer({ server, path })` with no Origin check and no token. Browsers do not apply same-origin to WebSockets, so any web page open in the owner's browser can now create, edit, delete and comment on beads, and could answer cards once D1 exists. Before any card, report or phone-door work ships: refuse an upgrade whose Origin is not the door's own origin; require a random per-launch token (served in the page) on every mutation and on `/api/report`; refuse to start the phone door unless bound to loopback. Blocking for phase 2, worth doing now regardless.

## 7. Cost
- Per active user per month: $0. Internal tool on the PC; Tailscale Personal free tier (1 of 3 users, 2 of 100 devices); GitHub private repo for images inside the free plan.
- Per idle user: $0 infrastructure; model cost zero turns while waiting, plus one re-arm turn per 8 h per open CoS session. Idle CPU of `wait` and of the second listener: SRE measures on the wake bead.
- Per event: one CoS turn plus the run the work needs anyway; capped at 30/h/project. Per report: about 250 KB stored and backed up.
- First limit we hit: the attachments repo at about 4,000 reports per GB (roll to a new repo); then the 3-user Tailscale limit only if testers join the tailnet.

## 8. Success condition for phase 2
From the phone over Tailscale: a report with a screenshot and text becomes a bead in the chosen project with the image visible on it, the image is in the attachments repo's backup within the hour, and the project's session is woken once; a tap card answered on the board is recorded (`owner-board`), returns to card_back, and a fresh run of that role starts within one minute with no owner step; a question on a part or option is recorded, the asker is woken, and its answer shows on the card; a desk card offers no answer on either door and the core refuses a forged one; the phone door hides desk beads; a board-created task wakes the project's session once and an agent-created bead wakes nobody; with no session open the event is shown as queued and delivered when a session arms `wait`; a foreign Origin, a missing token, or a request without the owner's Tailscale identity is refused.

## 9. Exit path
Plain JSON files, a git repo of images, Node scripts. If the board is replaced, the new UI calls the same card CLI and report handler contract. If Tailscale changes terms: WireGuard or headscale in a day. If Claude Code ships Channels generally, a channel plugin reads the same `pending/` folder instead of `wait`. Nothing to migrate off a provider.
