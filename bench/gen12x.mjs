// 12x generator: replicate a store's export (JSONL) N times with fresh ids, so beads, comments,
// dependencies and labels all scale in proportion. Output is plain JSONL for `bd import`
// (works for upstream and the fork). Usage:
//   node gen12x.mjs --in export.jsonl --out export12x.jsonl [--factor 12]
// Source export: `bd export -o export.jsonl` run in a COPY of a store.
import { readFileSync, writeFileSync } from "node:fs";
import { parseArgs } from "./lib.mjs";

const a = parseArgs(process.argv.slice(2));
const factor = Number(a.factor ?? 12);
const lines = readFileSync(a.in, "utf8").split(/\r?\n/).filter(Boolean);
const rows = lines.map((l) => JSON.parse(l));
const issues = rows.filter((r) => r._type !== "memory" && r.id);
const ids = new Set(issues.map((r) => r.id));
// longest first so "x-ab.1" is matched before "x-ab"
const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const alt = [...ids].sort((x, y) => y.length - x.length).map(esc).join("|");
const re = new RegExp(`(?<![A-Za-z0-9_.-])(${alt})(?![A-Za-z0-9_-])`, "g");

const out = [];
let n = 0, nc = 0, nd = 0, nl = 0;
for (let k = 0; k < factor; k++) {
  const tag = k === 0 ? "" : `z${k.toString(36)}`;
  for (let i = 0; i < lines.length; i++) {
    const obj = rows[i];
    if (obj._type === "memory") { if (k === 0) out.push(lines[i]); continue; }
    out.push(k === 0 ? lines[i] : lines[i].replace(re, (m) => m + tag));
    n++;
    nc += (obj.comments || []).length; nd += (obj.dependencies || []).length; nl += (obj.labels || []).length;
  }
}
writeFileSync(a.out, out.join("\n") + "\n");
console.log(JSON.stringify({ factor, source_beads: issues.length, beads: n, comments: nc, dependencies: nd, labels: nl, out: a.out }));
