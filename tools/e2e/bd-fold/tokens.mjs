// Independent token re-measure (QA, beads-lite-1bo5.6.46.2/.5). Picks 20 beads by sha1(id+seed) across several db copies
// (NOT by "most comment text" like scripts/bd-token-sample.mjs), runs prod bd and the new exe, bytes/4.
// Also scans for lost verdict / Next lines. Usage: node tokens.mjs <outdir>   env: BD_NEW BD_OLD QA_ENV
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { writeFileSync } from "node:fs";
import path from "node:path";

const NEW = process.env.BD_NEW, OLD = process.env.BD_OLD || "bd", ENV = process.env.QA_ENV, OUT = process.argv[2];
const SEED = "qa-1bo5.6.46.5";
const run = (bin, args, cwd) => spawnSync(bin, args, { cwd, encoding: "utf8", windowsHide: true, maxBuffer: 256e6, env: { ...process.env, BD_ACTOR: "qa", NO_COLOR: "1" } }).stdout || "";
const norm = (s) => s.split(/\s+/).filter(Boolean).join(" ");
const projects = ["beads-lite", "learner", "workout-tracker", "filmmaker", "patient-record", "abduu-tasks", "brother-exams"];
const swePicked = new Set(); // SWE's picks are "most comment text"; we exclude the top-20 by comment weight in beads-lite to be independent
const pool = [];
for (const p of projects) {
  const cwd = path.join(ENV, p);
  let list; try { list = JSON.parse(run(OLD, ["list", "--status", "closed", "--limit", "600", "--json"], cwd)); } catch { continue; }
  for (const it of list) pool.push({ p, cwd, id: it.id });
}
// need comments count: use show --json --include-comments --full only for candidates, by hash order
const h = (id) => createHash("sha1").update(id + SEED).digest("hex");
pool.sort((a, b) => (h(a.id) < h(b.id) ? -1 : 1));
const picked = [];
const perProject = {};
for (const c of pool) {
  if (picked.length >= 20) break;
  if ((perProject[c.p] || 0) >= 4) continue;
  const j = JSON.parse(run(OLD, ["show", c.id, "--json", "--full", "--include-comments"], c.cwd));
  const b = Array.isArray(j) ? j[0] : j; const cs = b.comments || [];
  if (cs.length < 3) continue; // a bead worth measuring has a comment thread
  c.cs = cs; c.b = b; picked.push(c); perProject[c.p] = (perProject[c.p] || 0) + 1;
}
const rows = []; let tProd = 0, tOld = 0, tNew = 0, tFull = 0, tNewFull = 0, worst = 0, vTot = 0, vOk = 0; const lost = [];
for (const c of picked) {
  const pd = process.env.BD_PROD ? run(process.env.BD_PROD, ["show", c.id], c.cwd) : ""; tProd += Math.round(Buffer.byteLength(pd)/4);
  const od = run(OLD, ["show", c.id], c.cwd), nd = run(NEW, ["show", c.id], c.cwd);
  const of = run(OLD, ["show", c.id, "--full"], c.cwd), nf = run(NEW, ["show", c.id, "--full"], c.cwd);
  const q = (s) => Math.round(Buffer.byteLength(s) / 4);
  tOld += q(od); tNew += q(nd); tFull += q(of); tNewFull += q(nf); worst = Math.max(worst, q(nd));
  const out = norm(nd);
  const shown = c.cs.slice(-3);
  for (const k of shown) {
    const t = k.text || ""; const n = norm(t);
    const sectioned = /^## /m.test(t);
    // verdict-bearing tail: last 60 runes (flat). For sectioned: the Verdict/Next/Result lines.
    if (!sectioned) { vTot++; if (out.includes([...n].slice(-60).join(""))) vOk++; else lost.push({ id: c.id, kind: "flat-tail", tail: [...n].slice(-60).join("") }); }
    else {
      const m = t.match(/^## *(verdict|next|result|pass|fail|decision|status|outcome)[^\n]*\n([^\n#][^\n]*)/im);
      const nextLine = t.match(/^(Next|Verdict|Result)[:：][^\n]*/im);
      if (m || nextLine) { vTot++; const probe = norm((m ? m[2] : nextLine[0])).slice(0, 40); if (out.includes(probe)) vOk++; else lost.push({ id: c.id, kind: "sectioned-verdict", section: m ? m[1] : "line", probe }); }
    }
  }
  rows.push({ id: c.id, project: c.p, comments: c.cs.length, oldDefault: q(od), newDefault: q(nd), oldFull: q(of), newFull: q(nf), fullSame: of === nf });
}
const res = { tProd, picked: picked.length, tOld, tNew, tFull, tNewFull, pctOfFull: +(tNew / tFull * 100).toFixed(1), deltaVsOld: +((tNew - tOld) / tOld * 100).toFixed(1), worst, verdictChecked: vTot, verdictShown: vOk, lost, rows };
writeFileSync(path.join(OUT, "tokens.json"), JSON.stringify(res, null, 1));
const md = [`# Independent token sample (seed ${SEED}, ${picked.length} beads, bytes/4)`, "", "| bead | proj | cmts | old default | new default | full (old=new?) |", "|--|--|--|--|--|--|",
  ...rows.map((r) => `| ${r.id} | ${r.project} | ${r.comments} | ${r.oldDefault} | ${r.newDefault} | ${r.oldFull} (${r.fullSame}) |`), "",
  `TOTAL production-bd default ${tProd}, base-commit default ${tOld}, new default ${tNew} (${res.deltaVsOld}%), --full ${tFull}; new/full ${res.pctOfFull}%; worst bead ${worst}`,
  `Verdict/Next probes shown: ${vOk}/${vTot}`, "", "Lost:", ...lost.map((l) => "- " + JSON.stringify(l))];
writeFileSync(path.join(OUT, "tokens.md"), md.join("\n"));
console.log(md.join("\n"));
