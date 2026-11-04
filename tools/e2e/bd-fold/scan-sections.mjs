// How many comments (all dbs, newest 3 per bead = what default show prints) are sectioned and hide a Verdict/Next/Result section?
import { spawnSync } from "node:child_process";
import path from "node:path";
const OLD = process.env.BD_OLD, ENV = process.env.QA_ENV;
const run = (args, cwd) => spawnSync(OLD, args, { cwd, encoding: "utf8", windowsHide: true, maxBuffer: 512e6, env: { ...process.env, BD_ACTOR: "qa" } }).stdout || "";
const res = [];
for (const p of ["beads-lite", "learner", "workout-tracker", "filmmaker", "patient-record", "abduu-tasks", "brother-exams"]) {
  const cwd = path.join(ENV, p);
  let list; try { list = JSON.parse(run(["list", "--all", "--limit", "0", "--json"], cwd)); } catch { continue; }
  let beads = 0, shown = 0, sectioned = 0, hidesVerdict = 0, lastSecIsVerdict = 0;
  for (const it of list) {
    let j; try { j = JSON.parse(run(["show", it.id, "--json", "--full", "--include-comments"], cwd)); } catch { continue; }
    const b = Array.isArray(j) ? j[0] : j; const cs = (b.comments || []).slice(-3);
    if (!cs.length) continue; beads++;
    for (const c of cs) {
      shown++;
      const t = c.text || ""; if (!/^## /m.test(t)) continue; sectioned++;
      const heads = [...t.matchAll(/^## *(.+)$/gm)].map((m) => m[1].toLowerCase());
      if (heads.some((h) => /verdict|next|result|pass|fail|decision|outcome|ruling|recommend/.test(h))) hidesVerdict++;
      if (/verdict|next|result|pass|fail|decision|outcome|ruling|recommend/.test(heads[heads.length - 1] || "")) lastSecIsVerdict++;
    }
  }
  res.push({ project: p, beadsWithComments: beads, commentsShownByDefault: shown, sectioned, sectionedWithVerdictLikeSection: hidesVerdict, lastSectionIsVerdictLike: lastSecIsVerdict });
}
console.log(JSON.stringify(res, null, 1));
