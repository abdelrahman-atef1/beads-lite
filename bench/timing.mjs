#!/usr/bin/env node
// Timing harness for plan section 4 ("Success timings"). Whole-process wall time, median of N,
// optionally with a background load of K parallel bd readers (the agent load the targets assume).
// Runs against a SCRATCH copy of a pristine store; never against a live store.
//
//   node bench/timing.mjs [--project company] [--n 10] [--load 5] [--label upstream-1.3.1]
//        [--store <dir containing .beads>]   # e.g. the 12x store: skips the copy and uses it as is
//        [--no-write]  [--keep]
//   BD_BIN=<fork exe> node bench/timing.mjs --label fork     # same table for the fork
//
// Output: table on stdout + <corpus>/bench/<label>-<project>-<timestamp>.json
import fs from "node:fs";
import path from "node:path";
import { spawn, spawnSync } from "node:child_process";
import { CORPUS_DEFAULT, runBd, ensureDir, parseArgs, parseJson, median, bdBin, killTree } from "../parity/lib.mjs";

const o = parseArgs(process.argv.slice(2));
const corpus = o.corpus || CORPUS_DEFAULT;
const project = o.project || "company";
const label = o.label || "upstream-1.3.1";
const N = Number(o.n || 10);
const LOAD = Number(o.load ?? 0);
const stamp = new Date().toISOString().replace(/[:.]/g, "-");

// Targets from plan-draft.md section 4 (ms). 12x set uses its own row set.
const TARGET = { version: 80, show: 150, list: 200, ready: 200, write: 120, board: 1000, "12x.list": 400, "12x.ready": 250, "12x.show": 150 };
const is12x = !!o.store && /12x/.test(String(o.store));

let root, ownScratch = false;
if (o.store) root = String(o.store);
else {
  root = path.join(corpus, "scratch", `bench-${label}-${project}-${stamp}`);
  ensureDir(path.join(root, ".beads"));
  const src = path.join(corpus, "stores-pristine", project, ".beads");
  const rc = spawnSync("robocopy", [src, path.join(root, ".beads"), "/E", "/XD", "backup", "/XF", "*.lock", ".~*", "/NFL", "/NDL", "/NJH", "/NJS", "/NP"], { windowsHide: true });
  if (rc.status > 7) throw new Error("robocopy failed " + rc.status);
  ownScratch = true;
}

async function timeIt(args, n = N) {
  const ms = [];
  let code = 0;
  for (let i = 0; i < n; i++) {
    const r = await runBd(root, args, { timeoutMs: 600000 });
    ms.push(r.ms);
    if (r.code !== 0) code = r.code;
  }
  return { ms, median: median(ms), min: Math.min(...ms), max: Math.max(...ms), code };
}

// ---- background load: K loops of a read command; tracked by PID, stopped by PID ----
const loaders = [];
function startLoad(k) {
  const env = { ...process.env, BEADS_DIR: path.join(root, ".beads"), BD_DISABLE_METRICS: "1", BD_ACTOR: "qa-bench-load" };
  for (let i = 0; i < k; i++) {
    // a node child that loops `bd ready --json` until killed
    const code = `const {spawnSync}=require("child_process");while(true){spawnSync(process.argv[1],["ready","--json"],{cwd:process.argv[2],env:process.env,windowsHide:true,stdio:"ignore"});}`;
    const c = spawn(process.execPath, ["-e", code, bdBin(), root], { env, windowsHide: true, stdio: "ignore" });
    loaders.push(c);
  }
}
function stopLoad() { for (const c of loaders) killTree(c.pid); loaders.length = 0; }
process.on("exit", stopLoad);

// ---- pick a representative id: the one with the most comments + deps ----
const lst = await runBd(root, ["list", "--all", "--limit", "0", "--json"]);
const all = parseJson(lst.stdout) || [];
const total = all.length;
const heavy = [...all].sort((a, b) => ((b.comment_count || 0) + (b.dependency_count || 0)) - ((a.comment_count || 0) + (a.dependency_count || 0)))[0];
const showId = heavy?.id;
console.log(`bd=${bdBin()}  label=${label}  store=${root}  beads=${total}  heavy-id=${showId}  n=${N}  load=${LOAD}`);

if (LOAD) { startLoad(LOAD); await new Promise((r) => setTimeout(r, 2000)); }

const rows = [];
const add = (key, desc, t, extra = {}) => rows.push({ key, desc, ...t, target: TARGET[key], ok: TARGET[key] == null ? null : t.median <= TARGET[key], ...extra });

const pre = is12x ? "12x." : "";
if (!is12x) add("version", "bd version", await timeIt(["version"]));
add(is12x ? "12x.show" : "show", `show ${showId} --json`, await timeIt(["show", showId, "--json"]));
add(is12x ? "12x.list" : "list", "list --json (default)", await timeIt(["list", "--json"]));
add(is12x ? "12x.ready" : "ready", "ready --json", await timeIt(["ready", "--json"]));

if (!is12x) {
  // Board full load: the 7 subscriptions + one detail, as beads-ui issues them (ui-calls.md)
  const boardCmds = [
    ["ready", "--limit", "1000", "--json"],
    ["blocked", "--json"],
    ["list", "--json", "--tree=false", "--status", "in_progress", "--limit", "0"],
    ["list", "--json", "--tree=false", "--status", "owner_decision", "--limit", "0"],
    ["list", "--json", "--tree=false", "--status", "waiting_for_test", "--limit", "0"],
    ["list", "--json", "--tree=false", "--status", "waiting_for_merge", "--limit", "0"],
    ["list", "--json", "--tree=false", "--status", "closed", "--limit", "1000"],
    ["show", showId, "--json", "--include-dependents"],
  ];
  const reps = Math.max(3, Math.min(N, 5));
  const totals = [];
  for (let i = 0; i < reps; i++) {
    let sum = 0;
    for (const c of boardCmds) { const r = await runBd(root, c, { timeoutMs: 600000 }); sum += r.ms; }
    totals.push(sum);
  }
  add("board", `board full load: ${boardCmds.length} sequential bd calls (UI serialises them)`, { ms: totals, median: median(totals), min: Math.min(...totals), max: Math.max(...totals), code: 0 });

  if (!o["no-write"]) {
    // Writes on scratch only. update, comment, close(create first, untimed).
    const w = [];
    const wid = `${all[0].id.split("-")[0]}-qabw`;
    await runBd(root, ["create", "bench write target", "--id", wid, "--silent"]);
    for (let i = 0; i < N; i++) w.push((await runBd(root, ["update", wid, "--set-metadata", `bench=${i}`])).ms);
    add("write", "update --set-metadata (write)", { ms: w, median: median(w), min: Math.min(...w), max: Math.max(...w), code: 0 }, { variant: "update" });
    const c = [];
    for (let i = 0; i < N; i++) c.push((await runBd(root, ["comments", "add", wid, `bench comment ${i}`])).ms);
    rows.push({ key: "write.comment", desc: "comments add (write)", ms: c, median: median(c), min: Math.min(...c), max: Math.max(...c), target: TARGET.write, ok: median(c) <= TARGET.write });
    const cl = [];
    for (let i = 0; i < Math.min(N, 5); i++) {
      const id = `${wid}${i}`;
      await runBd(root, ["create", "bench close target", "--id", id, "--silent"]);
      cl.push((await runBd(root, ["close", id, "--reason", "bench", "--force"])).ms);
    }
    rows.push({ key: "write.close", desc: "close --reason --force (write)", ms: cl, median: median(cl), min: Math.min(...cl), max: Math.max(...cl), target: TARGET.write, ok: median(cl) <= TARGET.write });
  }
}
stopLoad();

console.log("\n" + "key".padEnd(14) + "median".padStart(9) + "min".padStart(9) + "max".padStart(9) + "target".padStart(9) + "  result  command");
for (const r of rows)
  console.log(r.key.padEnd(14) + String(r.median).padStart(9) + String(r.min).padStart(9) + String(r.max).padStart(9) + String(r.target ?? "-").padStart(9) + "  " + (r.ok == null ? "  -   " : r.ok ? " PASS  " : " FAIL  ") + r.desc);
ensureDir(path.join(corpus, "bench"));
const outFile = path.join(corpus, "bench", `${label}-${project}${is12x ? "-12x" : ""}-${stamp}.json`);
fs.writeFileSync(outFile, JSON.stringify({ label, bd: bdBin(), project, beads: total, n: N, load: LOAD, root, rows }, null, 1));
console.log("\nresults:", outFile);
if (ownScratch && !o.keep) fs.rmSync(root, { recursive: true, force: true });
