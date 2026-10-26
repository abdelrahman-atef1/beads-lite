#!/usr/bin/env node
// Parity oracle capture: run the command matrix against copies of the real project stores
// with whichever bd binary BD_BIN points at (default: installed upstream bd 1.3.1) and save
// raw stdout per command. Re-run with BD_BIN=<fork> and --label fork to get the other side,
// then `node parity/compare.mjs`.
//
//   node parity/capture.mjs [--corpus E:/Projects/beads-lite-corpus] [--label upstream-1.3.1]
//        [--projects whisper-flow,company] [--pconc 2] [--conc 1] [--force] [--only <prefix>]
//
// The corpus (store copies + oracle outputs) holds real bead data and lives OUTSIDE this repo.
import fs from "node:fs";
import { spawnSync } from "node:child_process";
import path from "node:path";
import {
  PROJECTS, CORPUS_DEFAULT, runBd, storeRoot, ensureDir, parseJson, pool, sha, parseArgs, bdBin,
} from "./lib.mjs";
import { buildCtx, matrix } from "./commands.mjs";

const o = parseArgs(process.argv.slice(2));
const corpus = o.corpus || CORPUS_DEFAULT;
const label = o.label || "upstream-1.3.1";
const projects = o.projects ? String(o.projects).split(",") : PROJECTS;
const pconc = Number(o.pconc || 2);
const conc = Number(o.conc || 1);
const timeoutMs = Number(o.timeout || 900000);

// Extra worker stores: embedded Dolt allows one process per store, so --conc N uses N copies
// (copy 0 = stores/<p>, others copied from stores-pristine into stores-w/<p>-<k>).
function workerRoots(p) {
  const roots = [storeRoot(corpus, p)];
  for (let k = 1; k < conc; k++) {
    const r = path.join(corpus, "stores-w", `${p}-${k}`);
    if (!fs.existsSync(path.join(r, ".beads", "metadata.json"))) {
      ensureDir(path.join(r, ".beads"));
      const rc = spawnSync("robocopy", [path.join(corpus, "stores-pristine", p, ".beads"), path.join(r, ".beads"), "/E", "/XD", "backup", "/XF", "*.lock", ".~*", "/NFL", "/NDL", "/NJH", "/NJS", "/NP"], { windowsHide: true });
      if (rc.status > 7) throw new Error("robocopy failed " + rc.status);
    }
    roots.push(r);
  }
  return roots;
}

async function doProject(p) {
  const root = storeRoot(corpus, p);
  const outDir = path.join(corpus, "oracle", label, p);
  ensureDir(outDir);
  const log = (...a) => console.log(`[${p}]`, ...a);

  // Bootstrap: the full id list comes from the oracle itself (list --all).
  const allFile = path.join(outDir, "list.all.unlimited.out");
  let all;
  if (!o.force && fs.existsSync(allFile)) all = parseJson(fs.readFileSync(allFile, "utf8"));
  if (!all) {
    const r = await runBd(root, ["list", "--all", "--limit", "0", "--json"], { timeoutMs });
    if (r.code !== 0) { log("bootstrap failed", r.code, r.stderr.slice(0, 300)); return { p, failed: 1 }; }
    fs.writeFileSync(allFile, r.stdout);
    all = parseJson(r.stdout);
  }
  if (!Array.isArray(all)) { log("bootstrap: list --all did not return an array"); return { p, failed: 1 }; }
  log(`${all.length} beads`);

  const ctx = buildCtx(all);
  fs.writeFileSync(path.join(outDir, "_ctx.json"), JSON.stringify({ ...ctx, ids: undefined, idCount: ctx.ids.length, commentIds: ctx.commentIds.slice(0, 40) }, null, 1));
  let entries = matrix(ctx);
  if (o.only) entries = entries.filter((e) => e.name.startsWith(o.only));

  const idxFile = path.join(outDir, "_index.jsonl");
  const done = new Set();
  if (!o.force && fs.existsSync(idxFile))
    for (const l of fs.readFileSync(idxFile, "utf8").split("\n").filter(Boolean)) done.add(JSON.parse(l).name);

  let n = 0, bad = 0;
  const roots = workerRoots(p);
  const free = [...roots];
  const todo = entries.filter((e) => !done.has(e.name));
  await pool(todo, conc, async (e) => {
    const wroot = free.pop();
    try {
    const args = e.args.map((a) => a.replace("{OUT}", outDir.replace(/\\/g, "/")));
    let r = await runBd(wroot, args, { timeoutMs });
    // The shared PC is often out of memory: that is not a result. Retry with back-off.
    for (let t = 1; t <= 8 && /cannot allocate memory|out of memory|insufficient system resources|1455/i.test(r.stderr + r.stdout.slice(0, 400)); t++) {
      log("resource exhaustion, retry", t, e.name);
      await new Promise((res) => setTimeout(res, 45000));
      r = await runBd(wroot, args, { timeoutMs });
    }
    const outFile = path.join(outDir, e.name + ".out");
    fs.writeFileSync(outFile, r.stdout);
    let stdoutFile = e.name + ".out";
    if (e.kind === "file" && e.file && fs.existsSync(path.join(outDir, e.file))) stdoutFile = e.file;
    const rec = {
      name: e.name, kind: e.kind, args: e.args, code: r.code, ms: r.ms, timedOut: r.timedOut,
      bytes: Buffer.byteLength(r.stdout), stdoutSha: sha(r.stdout), file: stdoutFile,
      stderr: r.stderr.slice(0, 400), stderrSha: sha(r.stderr),
      jsonOk: e.kind === "json" ? parseJson(r.stdout) !== null : undefined,
    };
    fs.appendFileSync(idxFile, JSON.stringify(rec) + "\n");
    n++;
    if (r.code !== 0 && !e.name.startsWith("err.")) { bad++; log("nonzero", e.name, r.code, r.stderr.slice(0, 120).replace(/\n/g, " ")); }
    if (n % 10 === 0) log(`${n}/${todo.length} done`);
    } finally { free.push(wroot); }
  });
  log(`finished: ${n} run, ${done.size} resumed, ${bad} unexpected non-zero`);
  return { p, n, bad };
}

console.log(`bd under test: ${bdBin()} label=${label} corpus=${corpus}`);
const res = await pool(projects, pconc, doProject);
console.log(JSON.stringify(res));
