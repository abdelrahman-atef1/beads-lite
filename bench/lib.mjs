// Shared helpers for the beads-lite timing and stress harness (T004). No dependencies.
import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { join } from "node:path";

export function parseArgs(argv) {
  const o = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith("--")) {
      const k = a.slice(2);
      const v = argv[i + 1] !== undefined && !argv[i + 1].startsWith("--") ? argv[++i] : "true";
      o[k] = v;
    } else o._.push(a);
  }
  return o;
}

// Writes only ever go to a COPY of a store: the project root must hold a file named .bench-copy.
export function requireCopy(project) {
  if (!project || !existsSync(join(project, ".beads"))) throw new Error(`no .beads under ${project}`);
  if (!existsSync(join(project, ".bench-copy")))
    throw new Error(`refusing ${project}: not marked as a copy (create an empty .bench-copy file in it)`);
}

// Run one process by full path, hidden window; returns whole-process wall time in ms.
export function run(bin, args, { cwd, env, input, timeoutMs = 300000 } = {}) {
  return new Promise((resolve) => {
    const t0 = process.hrtime.bigint();
    const p = spawn(bin, args, { cwd, env: { ...process.env, ...env }, windowsHide: true, stdio: ["pipe", "pipe", "pipe"] });
    let out = "", err = "";
    const kill = setTimeout(() => { try { p.kill(); } catch {} }, timeoutMs);
    p.stdout.on("data", (d) => (out += d));
    p.stderr.on("data", (d) => (err += d));
    p.on("error", (e) => { clearTimeout(kill); resolve({ code: -1, ms: 0, out, err: String(e) }); });
    p.on("close", (code) => {
      clearTimeout(kill);
      resolve({ code, ms: Number(process.hrtime.bigint() - t0) / 1e6, out, err });
    });
    if (input) p.stdin.write(input);
    p.stdin.end();
  });
}

export const median = (a) => { const s = [...a].sort((x, y) => x - y); const n = s.length; return n ? (n % 2 ? s[(n - 1) / 2] : (s[n / 2 - 1] + s[n / 2]) / 2) : null; };
export const pct = (a, q) => { const s = [...a].sort((x, y) => x - y); return s.length ? s[Math.min(s.length - 1, Math.floor(q * s.length))] : null; };
export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
export const r1 = (x) => (x == null ? null : Math.round(x * 10) / 10);
