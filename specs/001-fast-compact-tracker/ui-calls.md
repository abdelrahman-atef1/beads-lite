# beads-ui bd call map (global npm beads-ui 0.12.6, patched)
Root: %APPDATA%\npm\node_modules\beads-ui\server

## Page load
Board subscribes 7 lists (client main.bundle.js): blocked? ready, in-progress, owner_decision, waiting_for_test, waiting_for_merge (patch adds 3), closed, plus blocked (tab:board:*).
Each -> ws.js:768 fetchInitialListForSubscription -> list-adapters.js:151 fetchListForSubscription -> runBdJson (list-adapters.js:164).
Args (list-adapters.js): ready `bd ready --limit 1000 --json` (:43); blocked `bd blocked --json` (:25); in_progress/owner_decision/waiting_*: `bd list --json --tree=false --status X --limit 0` (:45-65); closed `... --status closed --limit 1000` (:67). Detail: `bd show ID --json --include-dependents` (:83), cached.
## Refresh/watch
watcher.js:72 fs.watch(db dir), debounce 250ms (:20), cooldown 1000ms (:21), ignores last-touched/*.lock (:~80).
-> ws.js:673 scheduleListRefresh (debounce 75ms, ws.js:47,:193) -> refreshAllActiveListSubscriptions (ws.js:163) -> Promise.all over EVERY active spec (:166) -> refreshAndPublish (:453) -> same bd list per column. So ~7 bd spawns per change event (+ open detail).
Mutations (ws.js:881-1459): runBd update + runBdJson show, then mutation gate (ws.js:65) -> full refresh once.
No polling interval (only 30s ws heartbeat, ws.js:600).
## Serialization
bd.js:15-19,223-273: global queue, one bd at a time (Dolt embedded crash avoidance); interactive priority over background (refresh = background). So the Promise.all is effectively serial: 7 x ~9-12s = 60-80s per refresh.
## Patch (scripts/bdui-patch-board.mjs)
Adds 3 server subscription types + 3 list cases (validators.js, list-adapters.js), renames columns/CSS to 7 cols (main.bundle.js, styles.css) = 3 extra bd spawns per refresh. bd.js: default 90s timeout (BDUI_BD_TIMEOUT_MS), taskkill tree, exit 124.
## Batch/replace points
1. ws.js:163-175 refreshAllActiveListSubscriptions: run ONE `bd list --json --tree=false --limit 0` (all statuses, incl. closed cap) and split by status client-side/in registry; then call registry.applyItems per spec. Cuts 7->1 spawn.
2. list-adapters.js:151 fetchListForSubscription: memoize a single all-issues result for ~1-2s across specs (shared promise) so the 7 initial subscribes and refreshes coalesce.
3. ws.js:180 scheduleListRefresh / watcher.js:20-21: raise debounce/cooldown; skip refresh if one is in flight (queue backlog otherwise stacks).
4. ready/blocked need their own semantics (bd ready, bd blocked) -- derive from all-issues + deps, or keep those 2.
5. ws.js:881+ mutation paths: skip post-mutation `show` and rely on refresh.
