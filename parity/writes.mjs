#!/usr/bin/env node
// Write-path oracle. Reads are covered by capture.mjs; this runs a fixed script of WRITES
// (every write command in bd-surface.md rows 2,3,5,6,8,9,11,14) against a scratch copy of
// one project's pristine store, then reads the touched beads back. Run with BD_BIN set to the
// fork to produce the other side. Compare with:
//   node parity/compare.mjs --a upstream-1.3.1 --b fork --projects writes-<project> --strip-time
//
//   node parity/writes.mjs [--project whisper-flow] [--label upstream-1.3.1] [--keep]
//
// Never touches a live store: it copies <corpus>/stores-pristine/<project>/.beads to
// <corpus>/scratch/writes-<label>-<project>/ first, and deletes only that folder at the end.
import fs from "node:fs";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { CORPUS_DEFAULT, runBd, ensureDir, parseArgs, sha, bdBin } from "./lib.mjs";

const o = parseArgs(process.argv.slice(2));
const corpus = o.corpus || CORPUS_DEFAULT;
const project = o.project || "whisper-flow";
const label = o.label || "upstream-1.3.1";
const scratch = path.join(corpus, "scratch", `writes-${label}-${project}`);
const outDir = path.join(corpus, "oracle", label, `writes-${project}`);
const src = path.join(corpus, "stores-pristine", project, ".beads");
const dst = path.join(scratch, ".beads");

if (fs.existsSync(scratch)) throw new Error(`scratch exists, remove it first: ${scratch}`);
ensureDir(dst);
// robocopy exit codes 0-7 are success.
const rc = spawnSync("robocopy", [src, dst, "/E", "/XD", "backup", "/XF", "*.lock", ".~*", "/NFL", "/NDL", "/NJH", "/NJS", "/NP"], { windowsHide: true });
if (rc.status > 7) throw new Error("robocopy failed " + rc.status);
ensureDir(outDir);
fs.writeFileSync(path.join(outDir, "_index.jsonl"), "");

const prefix = JSON.parse(fs.readFileSync(path.join(src, "metadata.json"), "utf8")).dolt_database || project;
const A = `${prefix}-qaw1`, B = `${prefix}-qaw2`, C = `${prefix}-qaw3`;
const bodyFile = path.join(scratch, "body.md");
const noteFile = path.join(scratch, "note.txt");
fs.writeFileSync(bodyFile, "## Problem\nParity probe body اختبار\nline two\n");
fs.writeFileSync(noteFile, "comment from file\nsecond line اختبار\n");

const steps = [
  ["create.A", ["create", "QA parity A", "--id", A, "-a", "qa", "-p", "1", "--body-file", bodyFile, "--json"]],
  ["create.B-child", ["create", "QA parity B child", "--id", B, "--parent", A, "-p", "2", "--json"]],
  ["create.C-silent", ["create", "QA parity C", "--id", C, "--silent"]],
  ["create.minted-silent", ["create", "QA parity minted", "--silent"]], // id is minted: normalised below
  ["update.claim", ["update", A, "--claim", "--json"]],
  ["update.claim-again", ["update", A, "--claim", "--json"]],
  ["update.design", ["update", A, "--design", "design اختبار text", "--json"]],
  ["update.set-metadata", ["update", A, "--set-metadata", "card_q=why?", "--set-metadata", "card_state=open", "--json"]],
  ["update.status", ["update", B, "-s", "waiting_for_test", "--json"]],
  ["update.if-status-ok", ["update", B, "-s", "owner_decision", "--if-status", "waiting_for_test", "--json"]],
  ["update.if-status-mismatch", ["update", B, "-s", "open", "--if-status", "closed", "--json"]],
  ["update.if-assignee-mismatch", ["update", A, "-p", "0", "--if-assignee", "nobody", "--json"]],
  ["update.assignee", ["update", C, "-a", "someone", "--json"]],
  ["comments.add-text", ["comments", "add", A, "first comment اختبار", "--json"]],
  ["comments.add-author", ["comments", "add", A, "-a", "qa-author", "second comment", "--json"]],
  ["comments.add-file", ["comments", "add", A, "-f", noteFile, "--json"]],
  ["label.add", ["label", "add", A, "qa-x", "--json"]],
  ["label.add-two", ["label", "add", B, "qa-x,qa-y", "--json"]],
  ["dep.add-blocks", ["dep", "add", C, A, "--json"]],
  ["blocked.after-dep", ["blocked", "--json"]],
  ["ready.after-dep", ["ready", "--limit", "1000", "--json"]],
  ["dep.remove", ["dep", "remove", C, A, "--json"]],
  ["label.remove", ["label", "remove", B, "qa-y", "--json"]],
  ["config.set", ["config", "set", "qa.parity.key", "v1"]],
  ["config.get", ["config", "get", "qa.parity.key"]],
  ["close.reason", ["close", B, "--reason", "done اختبار", "--force", "--json"]],
  ["close.again", ["close", B, "--reason", "twice", "--json"]],
  ["close.A-force", ["close", A, "--reason", "parity done", "--force", "--json"]],
  ["update.reopen", ["update", A, "-s", "open", "--json"]],
  ["err.update-missing", ["update", "nope-doesnotexist-1", "-s", "open", "--json"]],
  ["err.close-missing", ["close", "nope-doesnotexist-1", "--json"]],
  ["err.comments-add-missing", ["comments", "add", "nope-doesnotexist-1", "x", "--json"]],
  ["read.show-all", ["show", A, B, C, "--json"]],
  ["read.show-comments", ["show", A, "--include-comments", "--json"]],
  ["read.comments", ["comments", A, "--json"]],
  ["read.list-open", ["list", "--tree=false", "--status", "open", "--limit", "0", "--json"]],
  ["read.list-all-count", ["count", "--by-status", "--json"]],
  ["read.export", ["export"]],
];

let minted = null;
const idx = path.join(outDir, "_index.jsonl");
for (const [name, args] of steps) {
  const r = await runBd(scratch, args, { timeoutMs: 900000 });
  let out = r.stdout;
  if (name === "create.minted-silent") { minted = out.trim().split(/\s+/)[0]; }
  if (minted) out = out.split(minted).join("<MINTED>");
  fs.writeFileSync(path.join(outDir, name + ".out"), out);
  const kind = out.trim().startsWith("{") || out.trim().startsWith("[") ? "json" : "text";
  fs.appendFileSync(idx, JSON.stringify({
    name, kind, args: args.map((a) => (a.startsWith(scratch) ? "<scratch>/" + path.basename(a) : a)),
    code: r.code, ms: r.ms, bytes: Buffer.byteLength(out), stdoutSha: sha(out), file: name + ".out",
    stderr: r.stderr.slice(0, 300).split(scratch).join("<scratch>"), stderrSha: sha(r.stderr),
  }) + "\n");
  console.log(name.padEnd(30), r.code, r.ms + "ms");
}
if (!o.keep) {
  // only the folder this run created
  fs.rmSync(scratch, { recursive: true, force: true });
}
console.log("bd:", bdBin(), "out:", outDir);
