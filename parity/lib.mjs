// Shared helpers for the parity oracle and bench harness (P0.2).
// Zero dependencies; Node >= 20. Every child process is started hidden and by recorded PID.
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

export const PROJECTS = ["whisper-flow", "abduu-tasks", "fatura", "rayeh", "company"];

// Where the private corpus lives. Never inside the repo.
export const CORPUS_DEFAULT = process.env.PARITY_CORPUS || "E:/Projects/beads-lite-corpus";

// The binary under test. Default = installed upstream bd. Set BD_BIN to the fork to compare.
export function bdBin() {
  if (process.env.BD_BIN) return process.env.BD_BIN;
  // The npm "bd" is a .cmd shim that spawn() cannot run without a shell; use the real exe.
  const exe = path.join(process.env.APPDATA || "", "npm", "node_modules", "@beads", "bd", "bin", "bd.exe");
  return fs.existsSync(exe) ? exe : "bd";
}

export function sha(s) {
  return createHash("sha256").update(s).digest("hex").slice(0, 16);
}

export function parseArgs(argv) {
  const o = { _: [] };
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (a.startsWith("--")) {
      const k = a.slice(2);
      const v = argv[i + 1] && !argv[i + 1].startsWith("--") ? argv[++i] : true;
      o[k] = v;
    } else o._.push(a);
  }
  return o;
}

/**
 * Run bd against ONE store copy. Never touches a live store: the caller passes the
 * store root (the folder that CONTAINS .beads) and we pin BEADS_DIR to it.
 * Returns { code, stdout, stderr, ms, timedOut }.
 */
export function runBd(storeRoot, args, opts = {}) {
  const { timeoutMs = 600000, input, bin = bdBin(), extraEnv = {} } = opts;
  const beadsDir = path.join(storeRoot, ".beads");
  const env = {
    ...process.env,
    BEADS_DIR: beadsDir,
    BD_DISABLE_METRICS: "1",
    BD_ACTOR: "qa-parity",
    BEADS_ACTOR: "qa-parity",
    NO_COLOR: "1",
    ...extraEnv,
  };
  return new Promise((resolve) => {
    const t0 = process.hrtime.bigint();
    const child = spawn(bin, args, { cwd: storeRoot, env, windowsHide: true, shell: false });
    const out = [];
    const err = [];
    let timedOut = false;
    const timer = setTimeout(() => {
      timedOut = true;
      killTree(child.pid);
    }, timeoutMs);
    child.stdout.on("data", (d) => out.push(d));
    child.stderr.on("data", (d) => err.push(d));
    child.on("error", (e) => {
      clearTimeout(timer);
      resolve({ code: -1, stdout: "", stderr: String(e), ms: 0, timedOut: false });
    });
    child.on("close", (code) => {
      clearTimeout(timer);
      const ms = Number(process.hrtime.bigint() - t0) / 1e6;
      resolve({
        code: timedOut ? 124 : code,
        stdout: Buffer.concat(out).toString("utf8"),
        stderr: Buffer.concat(err).toString("utf8"),
        ms: Math.round(ms),
        timedOut,
      });
    });
    if (input != null) child.stdin.end(input);
    else child.stdin.end();
  });
}

// Kill only the process tree we started (by PID), never by name.
export function killTree(pid) {
  if (!pid) return;
  try {
    if (process.platform === "win32") {
      spawn("taskkill", ["/PID", String(pid), "/T", "/F"], { windowsHide: true, stdio: "ignore" });
    } else process.kill(pid, "SIGKILL");
  } catch {}
}

export function storeRoot(corpus, project) {
  return path.join(corpus, "stores", project);
}

export function ensureDir(d) {
  fs.mkdirSync(d, { recursive: true });
}

// Parse JSON from bd stdout; bd may print warnings on stderr only, but be tolerant of
// leading non-JSON lines.
export function parseJson(s) {
  const t = s.trim();
  if (!t) return null;
  try {
    return JSON.parse(t);
  } catch {
    const i = t.search(/[\[{]/);
    if (i >= 0) {
      try {
        return JSON.parse(t.slice(i));
      } catch {}
    }
    return null;
  }
}

// Bounded-concurrency map.
export async function pool(items, n, fn) {
  const res = new Array(items.length);
  let i = 0;
  await Promise.all(
    Array.from({ length: Math.min(n, items.length) }, async () => {
      while (i < items.length) {
        const k = i++;
        res[k] = await fn(items[k], k);
      }
    }),
  );
  return res;
}

export function median(a) {
  if (!a.length) return null;
  const s = [...a].sort((x, y) => x - y);
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : Math.round((s[m - 1] + s[m]) / 2);
}
