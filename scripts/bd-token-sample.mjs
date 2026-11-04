#!/usr/bin/env node
// Token sample for `bd show` (beads-lite-1bo5.6.46.2).
// Usage: node scripts/bd-token-sample.mjs [--bd <path>] [--n 20] [--cwd <repo>] [--label before|after]
// Picks the N closed beads with the most comment text (>=5 comments or a field >40 lines),
// runs `bd show <id>` (default output) and `bd show <id> --full`, prints bytes and tokens (bytes/4).
import { execFileSync } from "node:child_process";
const a = process.argv.slice(2);
const opt = (k, d) => (a.includes(k) ? a[a.indexOf(k) + 1] : d);
const bd = opt("--bd", "bd"), n = +opt("--n", 20), cwd = opt("--cwd", process.cwd()), label = opt("--label", "run");
const run = (args, max = 64e6) =>
  execFileSync(bd, args, { cwd, encoding: "utf8", windowsHide: true, shell: process.platform === "win32", maxBuffer: max, env: { ...process.env, BD_ACTOR: "swe" } });
const list = JSON.parse(run(["list", "--status", "closed", "--limit", "400", "--json"]));
const cand = [];
for (const it of list) {
  const j = JSON.parse(run(["show", it.id, "--json", "--full", "--include-comments"]));
  const b = Array.isArray(j) ? j[0] : j;
  const cs = b.comments || [];
  const lines = Math.max(...["description", "design", "notes"].map((f) => (b[f] || "").split("\n").length));
  if (cs.length >= 5 || lines > 40) cand.push({ id: it.id, w: cs.reduce((s, c) => s + (c.text || "").length, 0) });
}
cand.sort((x, y) => y.w - x.w);
let tot = { def: 0, full: 0 };
const rows = [];
let vOk = 0, vN = 0;
const norm = (x) => x.split(/\s+/).filter(Boolean).join(' ');
for (const c of cand.slice(0, n)) {
  const def = Buffer.byteLength(run(["show", c.id])), full = Buffer.byteLength(run(["show", c.id, "--full"]));
  if (vN < 10) {
    const j = JSON.parse(run(["show", c.id, "--json", "--full", "--include-comments"]));
    const cs = (Array.isArray(j) ? j[0] : j).comments || [];
    const out = norm(run(["show", c.id]));
    for (const k of cs.slice(-3)) { const t = norm(k.text || ""); if (/^##? /m.test(k.text || "")) continue; vN++; if (out.includes([...t].slice(-60).join(""))) vOk++; }
  }
  tot.def += def; tot.full += full;
  rows.push(`${c.id}\tdefault ${Math.round(def / 4)} tok\tfull ${Math.round(full / 4)} tok`);
}
console.log(`[${label}] bd=${bd} beads=${rows.length}`);
console.log(rows.join("\n"));
console.log(`verdict tails shown: ${vOk}/${vN}; max bead default ${Math.round(Math.max(...rows.map((r) => +r.match(/default (\d+)/)[1])))} tok`);
console.log(`TOTAL default ${Math.round(tot.def / 4)} tok, full ${Math.round(tot.full / 4)} tok, default/full ${(tot.def / tot.full * 100).toFixed(1)}%`);
