#!/usr/bin/env node
// Compare two oracle captures (e.g. upstream-1.3.1 vs fork). Gate G2 for CLI parity (W6)
// is: zero DIFF/CODE entries for every command a project script uses.
//
//   node parity/compare.mjs --a upstream-1.3.1 --b fork [--corpus ...] [--projects ...]
//        [--strip-time] [--skip version,export.file] [--verbose]
//
// Result classes per entry:
//   SAME   byte-equal stdout and equal exit code
//   NORM   equal after normalisation (CRLF, and with --strip-time ISO timestamps)
//   ORDER  JSON arrays hold the same elements in a different order (sorted-equal)
//   DIFF   different content   |  CODE  different exit code   |  MISSING  entry absent on one side
import fs from "node:fs";
import path from "node:path";
import { PROJECTS, CORPUS_DEFAULT, parseArgs, parseJson } from "./lib.mjs";

const o = parseArgs(process.argv.slice(2));
const corpus = o.corpus || CORPUS_DEFAULT;
const A = o.a || "upstream-1.3.1";
const B = o.b || "fork";
const projects = o.projects ? String(o.projects).split(",") : PROJECTS;
// Entries that legitimately differ between builds (version string/commit, absolute paths).
const skip = new Set(String(o.skip || "version,version.text").split(","));

const ISO = /\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})/g;
function norm(s) {
  let t = s.replace(/\r\n/g, "\n");
  if (o["strip-time"]) t = t.replace(ISO, "<ts>");
  return t;
}
function sortKey(x) { return JSON.stringify(x); }
function diffPaths(a, b, p = "$", out = [], max = 6) {
  if (out.length >= max) return out;
  if (typeof a !== typeof b || Array.isArray(a) !== Array.isArray(b) || (a === null) !== (b === null)) {
    out.push(`${p}: ${JSON.stringify(a)?.slice(0, 80)} != ${JSON.stringify(b)?.slice(0, 80)}`); return out;
  }
  if (a && typeof a === "object") {
    const ks = new Set([...Object.keys(a), ...Object.keys(b)]);
    for (const k of ks) {
      if (!(k in a)) out.push(`${p}.${k}: missing in A`);
      else if (!(k in b)) out.push(`${p}.${k}: missing in B`);
      else diffPaths(a[k], b[k], `${p}${Array.isArray(a) ? `[${k}]` : "." + k}`, out, max);
      if (out.length >= max) break;
    }
    return out;
  }
  if (a !== b) out.push(`${p}: ${JSON.stringify(a)?.slice(0, 80)} != ${JSON.stringify(b)?.slice(0, 80)}`);
  return out;
}
function readIndex(dir) {
  const f = path.join(dir, "_index.jsonl");
  const m = new Map();
  if (fs.existsSync(f)) for (const l of fs.readFileSync(f, "utf8").split("\n").filter(Boolean)) { const r = JSON.parse(l); m.set(r.name, r); }
  return m;
}

const report = { a: A, b: B, projects: {} };
let totalBad = 0;
for (const p of projects) {
  const da = path.join(corpus, "oracle", A, p), db = path.join(corpus, "oracle", B, p);
  const ia = readIndex(da), ib = readIndex(db);
  const names = new Set([...ia.keys(), ...ib.keys()]);
  const res = { SAME: 0, NORM: 0, ORDER: 0, DIFF: 0, CODE: 0, MISSING: 0, SKIPPED: 0, details: [] };
  for (const name of [...names].sort()) {
    if (skip.has(name)) { res.SKIPPED++; continue; }
    const ra = ia.get(name), rb = ib.get(name);
    if (!ra || !rb) { res.MISSING++; res.details.push({ name, cls: "MISSING", note: ra ? "B missing" : "A missing" }); continue; }
    if (ra.code !== rb.code) { res.CODE++; res.details.push({ name, cls: "CODE", note: `${ra.code} vs ${rb.code}`, stderrA: ra.stderr.slice(0, 120), stderrB: rb.stderr.slice(0, 120) }); continue; }
    const fa = fs.readFileSync(path.join(da, ra.file), "utf8"), fb = fs.readFileSync(path.join(db, rb.file), "utf8");
    if (fa === fb) { res.SAME++; continue; }
    if (norm(fa) === norm(fb)) { res.NORM++; continue; }
    if (ra.kind === "json") {
      const ja = parseJson(norm(fa)), jb = parseJson(norm(fb));
      if (Array.isArray(ja) && Array.isArray(jb) && ja.length === jb.length &&
          ja.map(sortKey).sort().join("\n") === jb.map(sortKey).sort().join("\n")) { res.ORDER++; res.details.push({ name, cls: "ORDER" }); continue; }
      res.DIFF++; res.details.push({ name, cls: "DIFF", paths: ja && jb ? diffPaths(ja, jb) : ["unparseable JSON on one side"] }); continue;
    }
    res.DIFF++; res.details.push({ name, cls: "DIFF", paths: ["text differs"] });
  }
  report.projects[p] = res;
  totalBad += res.DIFF + res.CODE + res.MISSING;
  console.log(`${p}: SAME=${res.SAME} NORM=${res.NORM} ORDER=${res.ORDER} DIFF=${res.DIFF} CODE=${res.CODE} MISSING=${res.MISSING} SKIPPED=${res.SKIPPED}`);
  if (o.verbose) for (const d of res.details) console.log("  ", d.cls, d.name, d.note || (d.paths || []).join(" | "));
}
const outFile = path.join(corpus, "oracle", `diff-${A}-vs-${B}.json`);
fs.writeFileSync(outFile, JSON.stringify(report, null, 1));
console.log(`report: ${outFile}; blocking differences: ${totalBad}`);
process.exit(totalBad ? 1 : 0);
