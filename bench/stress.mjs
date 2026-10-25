#!/usr/bin/env node
// Concurrency stress (plan section 4, last row): W parallel writers x M writes each.
// Pass = 0 "database is locked" (or any lock-timeout) errors, 0 failed writes, 0 lost writes.
// Half of each writer's writes go to its own bead, half to one shared bead, so there is
// both disjoint and contended traffic. Lost writes are detected by counting comments after.
//
//   node bench/stress.mjs [--writers 20] [--writes 50] [--project whisper-flow] [--label upstream-1.3.1]
//   BD_BIN=<fork exe> node bench/stress.mjs --label fork
//
// Scratch copy of the pristine store only. NOTE: on upstream (embedded Dolt, single-process lock)
// 1000 writes take a long time; use --writers 4 --writes 5 for a smoke run.
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { CORPUS_DEFAULT, runBd, ensureDir, parseArgs, parseJson, pool, bdBin } from "../parity/lib.mjs";

const o = parseArgs(process.argv.slice(2));
const corpus = o.corpus || CORPUS_DEFAULT;
const project = o.project || "whisper-flow";
const label = o.label || "upstream-1.3.1";
const W = Number(o.writers || 20), M = Number(o.writes || 50);
const stamp = new Date().toISOString().replace(/[:.]/g, "-");
const root = path.join(corpus, "scratch", `stress-${label}-${project}-${stamp}`);
ensureDir(path.join(root, ".beads"));
const rc = spawnSync("robocopy", [path.join(corpus, "stores-pristine", project, ".beads"), path.join(root, ".beads"), "/E", "/XD", "backup", "/XF", "*.lock", ".~*", "/NFL", "/NDL", "/NJH", "/NJS", "/NP"], { windowsHide: true });
if (rc.status > 7) throw new Error("robocopy failed " + rc.status);

const prefix = JSON.parse(fs.readFileSync(path.join(root, ".beads", "metadata.json"), "utf8")).dolt_database || project;
const shared = `${prefix}-qas0`;
await runBd(root, ["create", "stress shared", "--id", shared, "--silent"]);
for (let w = 1; w <= W; w++) await runBd(root, ["create", `stress writer ${w}`, "--id", `${prefix}-qas${w}`, "--silent"]);

const errors = [];
let okWrites = 0, locked = 0;
const t0 = Date.now();
await pool(Array.from({ length: W }, (_, i) => i + 1), W, async (w) => {
  for (let i = 0; i < M; i++) {
    const target = i % 2 === 0 ? `${prefix}-qas${w}` : shared;
    const r = await runBd(root, ["comments", "add", target, `w${w}-${i}`], { timeoutMs: 300000 });
    if (r.code === 0) okWrites++;
    else {
      const text = r.stderr + r.stdout;
      if (/locked|busy|timeout/i.test(text)) locked++;
      errors.push({ w, i, code: r.code, msg: text.slice(0, 160) });
    }
  }
});
const secs = (Date.now() - t0) / 1000;

// lost-write check: every OK write must be visible
let seen = 0;
for (const id of [shared, ...Array.from({ length: W }, (_, i) => `${prefix}-qas${i + 1}`)]) {
  const r = await runBd(root, ["comments", id, "--json"], { timeoutMs: 300000 });
  const j = parseJson(r.stdout);
  if (Array.isArray(j)) seen += j.filter((c) => /^w\d+-\d+$/.test(c.text || c.Text || "")).length;
}
const lost = okWrites - seen;
const res = { label, bd: bdBin(), project, writers: W, writes: M, expected: W * M, ok: okWrites, failed: errors.length, locked, lost, seconds: Math.round(secs), pass: locked === 0 && errors.length === 0 && lost === 0 };
console.log(JSON.stringify(res, null, 1));
if (errors.length) console.log("first errors:", JSON.stringify(errors.slice(0, 5), null, 1));
ensureDir(path.join(corpus, "bench"));
fs.writeFileSync(path.join(corpus, "bench", `stress-${label}-${project}-${stamp}.json`), JSON.stringify({ ...res, errors: errors.slice(0, 50) }, null, 1));
if (!o.keep) fs.rmSync(root, { recursive: true, force: true });
process.exit(res.pass ? 0 : 1);
