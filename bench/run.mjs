// Timing: whole-process median of N (default 20) per command, with optional background load.
// Usage: node run.mjs --bd <full path to bd.exe> --project <copy dir> --label upstream-today
//          [--n 20] [--agents 3] [--board-url http://127.0.0.1:PORT/] --out results/<name>.json
// "board open" = a poll loop hitting --board-url every 2 s (start the board yourself on the copy).
// "agents" = N loops of mixed read/write commands running the whole time.
import { writeFileSync } from "node:fs";
import { parseArgs, requireCopy, run, median, pct, sleep, r1 } from "./lib.mjs";

const a = parseArgs(process.argv.slice(2));
requireCopy(a.project);
const bd = a.bd, cwd = a.project, N = Number(a.n ?? 20), agents = Number(a.agents ?? 3);
const bdrun = (args, o = {}) => run(bd, args, { cwd, ...o });

let stop = false;
const bg = [];
if (a["board-url"]) bg.push((async () => { while (!stop) { try { await fetch(a["board-url"]); } catch {} await sleep(2000); } })());

const lst = await bdrun(["list", "--all", "--json", "--limit", "0"]);
let ids = [];
try { ids = JSON.parse(lst.out).map((x) => x.id); } catch {}
if (!ids.length) { console.error("cannot list beads:", lst.err.slice(0, 300)); process.exit(2); }
const target = ids[Math.floor(ids.length / 2)];
const mine = []; // beads created by this run, closed at the end

for (let i = 0; i < agents; i++) bg.push((async () => {
  let j = 0;
  while (!stop) {
    const id = ids[(i * 97 + j * 31) % ids.length];
    const c = [["show", id, "--json"], ["list", "--json"], ["ready", "--json"], ["comments", id, "--json"]][j % 4];
    await bdrun(c, { env: { BD_ACTOR: `bench-agent-${i}` } });
    j++;
    if (j % 5 === 0) await bdrun(["comments", "add", id, `bench agent ${i} note ${j}`], { env: { BD_ACTOR: `bench-agent-${i}` } });
  }
})());
if (bg.length) await sleep(3000);

const cmds = {
  version: () => ["version"],
  show: () => ["show", target, "--json"],
  list: () => ["list", "--json"],
  ready: () => ["ready", "--json"],
  comments: () => ["comments", target, "--json"],
  count: () => ["count"],
  create: () => ["create", "bench item", "--silent", "-p", "4"],
  update: () => ["update", mine[0] ?? target, "--design", `bench ${Date.now()}`],
  "comments add": () => ["comments", "add", mine[0] ?? target, "bench comment"],
  close: () => ["close", mine.shift(), "--reason", "bench"],
};
const results = {};
const save = (done) => { if (a.out) writeFileSync(a.out, JSON.stringify({ label: a.label, bd, project: cwd, beads: ids.length, agents, board_polling: !!a["board-url"], n: N, at: new Date().toISOString(), complete: done, results }, null, 2)); };
for (const [name, mk] of Object.entries(cmds)) {
  const ms = [], codes = [];
  for (let i = 0; i < N; i++) {
    if (name === "close" && !mine.length) break;
    const r = await bdrun(mk(), { env: { BD_ACTOR: "bench" } });
    ms.push(r.ms); codes.push(r.code);
    if (name === "create" && r.code === 0) mine.push(r.out.trim().split(/\s+/)[0]);
  }
  results[name] = { n: ms.length, median_ms: r1(median(ms)), p95_ms: r1(pct(ms, 0.95)), max_ms: r1(Math.max(...ms)), failures: codes.filter((c) => c !== 0).length };
  console.log(name.padEnd(14), JSON.stringify(results[name]));
  save(false);
}
stop = true; await Promise.all(bg);
const doc = { label: a.label, bd, bd_version: (await run(bd, ["version"], { cwd })).out.trim().split("\n")[0], project: cwd, beads: ids.length,
  agents, board_polling: !!a["board-url"], n: N, at: new Date().toISOString(), results };
if (a.out) writeFileSync(a.out, JSON.stringify({ ...doc, complete: true }, null, 2));
