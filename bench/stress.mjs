// Stress: W parallel writers x M writes (create / update / comments add round robin), optionally
// with a backup command looping throughout. Reports failures, lost writes, slowest call.
// Pass (SC-004): 0 failures, 0 lost writes, max call <= 2000 ms.
// Usage: node stress.mjs --bd <exe> --project <copy dir> --writers 10 --writes 50
//          [--backup-cmd "backup snapshot"  (args passed to bd)] --out results/<name>.json
import { writeFileSync } from "node:fs";
import { parseArgs, requireCopy, run, median, pct, sleep, r1 } from "./lib.mjs";

const a = parseArgs(process.argv.slice(2));
requireCopy(a.project);
const bd = a.bd, cwd = a.project, W = Number(a.writers ?? 10), M = Number(a.writes ?? 50);
const tag = `stress-${Date.now().toString(36)}`;
const lat = [], fails = [], createdIds = [];
let expectedComments = 0, stop = false, backups = 0, backupFails = 0;

const backup = a["backup-cmd"] ? (async () => {
  while (!stop) { const r = await run(bd, a["backup-cmd"].split(" "), { cwd }); backups++; if (r.code !== 0) backupFails++; await sleep(500); }
})() : null;

await Promise.all(Array.from({ length: W }, (_, w) => (async () => {
  const env = { BD_ACTOR: `${tag}-w${w}` };
  let mine = null;
  for (let i = 0; i < M; i++) {
    const kind = i % 3;
    let args;
    if (kind === 0 || !mine) args = ["create", `${tag} w${w} #${i}`, "--silent", "-p", "4"];
    else if (kind === 1) args = ["update", mine, "--design", `${tag} ${i}`];
    else args = ["comments", "add", mine, `${tag} w${w} c${i}`];
    const r = await run(bd, args, { cwd, env });
    lat.push(r.ms);
    if (r.code !== 0) { fails.push({ w, i, args: args.slice(0, 2), code: r.code, err: r.err.slice(0, 200) }); continue; }
    if (args[0] === "create") { mine = r.out.trim().split(/\s+/)[0]; createdIds.push(mine); }
    if (args[0] === "comments") expectedComments++;
  }
})()));
stop = true; if (backup) await backup;

// lost-write check: read everything back
const found = new Set();
const lst = await run(bd, ["list", "--all", "--json", "--limit", "0"], { cwd });
try { for (const x of JSON.parse(lst.out)) if (String(x.title).startsWith(tag)) found.add(x.id); } catch {}
const lostCreates = createdIds.filter((id) => !found.has(id)).length;
let commentsSeen = 0;
for (const id of createdIds) {
  const r = await run(bd, ["comments", id, "--json"], { cwd });
  try { commentsSeen += JSON.parse(r.out).filter((c) => String(c.text ?? c.body ?? "").startsWith(tag)).length; } catch {}
}
const lostComments = expectedComments - commentsSeen;
const max = Math.max(...lat);
const doc = { bd, project: cwd, writers: W, writes_each: M, calls: lat.length, failures: fails.length, lost_creates: lostCreates, lost_comments: lostComments,
  median_ms: r1(median(lat)), p95_ms: r1(pct(lat, 0.95)), max_ms: r1(max), backup_cmd: a["backup-cmd"] ?? null, backup_runs: backups, backup_failures: backupFails,
  pass_sc004: fails.length === 0 && lostCreates === 0 && lostComments === 0 && max <= 2000, sample_failures: fails.slice(0, 5), at: new Date().toISOString() };
console.log(JSON.stringify(doc, null, 2));
if (a.out) writeFileSync(a.out, JSON.stringify(doc, null, 2));
