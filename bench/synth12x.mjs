#!/usr/bin/env node
// Build the "12x synthetic" set (plan section 4: 1,619 company beads x 12 = 19,428 beads).
// Takes the oracle's `export` of a project and clones every record N times with new ids,
// remapping dependency and comment references. Writes JSONL only; load it into a store with
// the binary under test:  node bench/synth12x.mjs --import   (bd init + bd import into
// <corpus>/stores-12x/<label>/ , a separate folder per binary because the formats differ).
//
//   node bench/synth12x.mjs [--project company] [--times 12] [--import] [--label upstream-1.3.1]
import fs from "node:fs";
import path from "node:path";
import { CORPUS_DEFAULT, runBd, ensureDir, parseArgs, bdBin } from "../parity/lib.mjs";

const o = parseArgs(process.argv.slice(2));
const corpus = o.corpus || CORPUS_DEFAULT;
const project = o.project || "company";
const times = Number(o.times || 12);
const label = o.label || "upstream-1.3.1";
const srcFile = path.join(corpus, "oracle", "upstream-1.3.1", project, "export.stdout.out");
const lines = fs.readFileSync(srcFile, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l));
const issues = lines.filter((r) => r.id && r._type !== "memory");
const outFile = path.join(corpus, "synthetic", `${project}-x${times}.jsonl`);
ensureDir(path.dirname(outFile));
const out = fs.createWriteStream(outFile);
const map = (id, k) => (k === 0 ? id : `${id}-s${k}`);
let n = 0;
for (let k = 0; k < times; k++) {
  for (const r of issues) {
    const c = JSON.parse(JSON.stringify(r));
    c.id = map(r.id, k);
    if (k) c.title = `${r.title} [copy ${k}]`;
    if (Array.isArray(c.dependencies)) for (const d of c.dependencies) {
      if (d.issue_id) d.issue_id = map(d.issue_id, k);
      if (d.depends_on_id && !String(d.depends_on_id).startsWith("external:")) d.depends_on_id = map(d.depends_on_id, k);
    }
    if (Array.isArray(c.comments)) for (const m of c.comments) {
      if (m.issue_id) m.issue_id = map(m.issue_id, k);
      if (k && m.id) m.id = undefined; // let the store mint comment ids
    }
    out.write(JSON.stringify(c) + "\n");
    n++;
  }
}
await new Promise((r) => out.end(r));
console.log(`wrote ${n} records -> ${outFile}`);

if (o.import) {
  const root = path.join(corpus, "stores-12x", label);
  if (fs.existsSync(root)) throw new Error("exists, remove it first: " + root);
  ensureDir(root);
  const prefix = (issues[0].id.match(/^(.*?)-/) || [])[1] || project;
  let r = await runBd(root, ["init", "--non-interactive", "--prefix", prefix, "--quiet"], { timeoutMs: 900000 });
  console.log("init", r.code, r.ms + "ms", r.stderr.slice(0, 200));
  r = await runBd(root, ["import", outFile], { timeoutMs: 3600000 });
  console.log("import", r.code, r.ms + "ms", (r.stdout + r.stderr).slice(0, 300));
  const c = await runBd(root, ["count", "--json"]);
  console.log("count", c.stdout.trim().replace(/\s+/g, " "), "via", bdBin());
}
