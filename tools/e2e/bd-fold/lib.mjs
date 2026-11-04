// Helpers for the bd-fold QA run (beads-lite-1bo5.6.46.2 / .5). No app code.
import { spawnSync } from "node:child_process";
import { cpSync, rmSync, mkdirSync, writeFileSync, existsSync } from "node:fs";
import path from "node:path";

export const NEW = process.env.BD_NEW;           // exe under test
export const OLD = process.env.BD_OLD || "bd";   // production bd (baseline, read only)
export const ENV = process.env.QA_ENV;           // folder holding db copies: <ENV>/<project>/.beads
export const FIX = path.join(ENV, "fix");        // writable fixture copy

export function run(bin, args, { cwd = FIX, actor = "qa", input, timeout = 60000 } = {}) {
  const t0 = Date.now();
  const r = spawnSync(bin, args, { cwd, input, windowsHide: true, timeout, maxBuffer: 256e6,
    env: { ...process.env, BD_ACTOR: actor, NO_COLOR: "1" } });
  const buf = r.stdout || Buffer.alloc(0);
  const tip = (x) => x.replace(/\n*💡 Tip:[^\n]*\n?/g, "\n"); // bd prints a random tip line: noise
  return { out: tip(buf.toString("utf8")), buf, err: (r.stderr || Buffer.alloc(0)).toString("utf8"), rc: r.status, ms: Date.now() - t0 };
}
export const nw = (args, o) => run(NEW, args, o);
export const ol = (args, o) => run(OLD, args, o);
export const validUtf8 = (buf) => { try { new TextDecoder("utf-8", { fatal: true }).decode(buf); return true; } catch { return false; } };
export const tokens = (s) => Math.round(Buffer.byteLength(s) / 4);

export function freshFixture(srcProject = "beads-lite") {
  rmSync(FIX, { recursive: true, force: true });
  mkdirSync(FIX, { recursive: true });
  cpSync(path.join(ENV, srcProject), FIX, { recursive: true });
}
export function create(title, { desc, design, notes, assignee } = {}) {
  const a = ["create", title, "-p", "3", "--json"];
  if (desc !== undefined) a.push("-d", desc);
  if (design !== undefined) a.push("--design", design);
  if (notes !== undefined) a.push("--notes", notes);
  if (assignee) a.push("-a", assignee);
  const r = nw(a);
  const m = r.out.match(/"id":\s*"([^"]+)"/);
  if (!m) throw new Error("create failed: " + r.out + r.err);
  return m[1];
}
export function addComment(id, text, actor = "qa", tmpdir) {
  const f = path.join(tmpdir, "c-" + Math.random().toString(36).slice(2) + ".txt");
  writeFileSync(f, text);
  const r = nw(["comments", "add", id, "-f", f], { actor });
  if (r.rc !== 0) throw new Error("comment add failed: " + r.out + r.err);
}
export function jsonComments(bin, id, cwd = FIX) {
  const r = run(bin, ["show", id, "--json", "--full", "--include-comments"], { cwd });
  const j = JSON.parse(r.out);
  return (Array.isArray(j) ? j[0] : j).comments || [];
}
