// The command matrix: every command in specs/001-fast-compact-tracker/bd-surface.md and
// ui-calls.md that READS a store. Writes are covered by writes.mjs (scratch copy, scripted).
// Each entry: { name, args, kind: "json"|"text"|"file", fileArg? }
// `ctx` is built from the store itself (ids, statuses, assignees, labels) so the matrix
// adapts to each project and stays deterministic for a given store snapshot.

export const BOARD_STATUSES = [
  "open", "in_progress", "owner_decision", "waiting_for_test",
  "waiting_for_merge", "blocked", "deferred", "closed",
];

const slug = (s) => String(s).replace(/[^A-Za-z0-9._-]+/g, "_").slice(0, 60);

// Pick sample ids that exercise shapes: most comments, most deps, most dependents,
// non-ASCII text, a parent with children, a closed one, an in-progress one.
export function pickSamples(all) {
  const by = (f, n) => [...all].sort((a, b) => f(b) - f(a)).slice(0, n).map((x) => x.id);
  const s = new Set();
  by((x) => x.comment_count || 0, 3).forEach((i) => s.add(i));
  by((x) => x.dependency_count || 0, 2).forEach((i) => s.add(i));
  by((x) => x.dependent_count || 0, 3).forEach((i) => s.add(i));
  const arabic = all.find((x) => /[؀-ۿ]/.test(x.title || ""));
  if (arabic) s.add(arabic.id);
  for (const st of ["closed", "in_progress", "open", "owner_decision"]) {
    const f = all.find((x) => x.status === st);
    if (f) s.add(f.id);
  }
  return [...s].slice(0, 12);
}

export function buildCtx(all) {
  const count = (f) => {
    const m = new Map();
    for (const x of all) for (const v of f(x)) m.set(v, (m.get(v) || 0) + 1);
    return [...m.entries()].sort((a, b) => b[1] - a[1] || String(a[0]).localeCompare(String(b[0]))).map((e) => e[0]);
  };
  const statuses = count((x) => [x.status]);
  const assignees = count((x) => (x.assignee ? [x.assignee] : []));
  const labels = count((x) => x.labels || []);
  const types = count((x) => [x.issue_type]);
  const parents = all
    .filter((x) => (x.dependent_count || 0) > 0 && (x.issue_type === "epic" || /\./.test(x.id) === false))
    .sort((a, b) => b.dependent_count - a.dependent_count)
    .map((x) => x.id);
  return {
    ids: all.map((x) => x.id).sort(),
    statuses, assignees, labels, types,
    parent: parents[0],
    samples: pickSamples(all),
    commentIds: [...all].filter((x) => (x.comment_count || 0) > 0)
      .sort((a, b) => b.comment_count - a.comment_count || a.id.localeCompare(b.id)).map((x) => x.id),
  };
}

export function matrix(ctx, { showBatch = 100, commentsSample = 40 } = {}) {
  const m = [];
  const J = (name, args) => m.push({ name, args: [...args, "--json"], kind: "json" });
  const T = (name, args) => m.push({ name, args, kind: "text" });

  // --- 14/13: global --json plumbing, count --------------------------------------------
  J("count", ["count"]);
  for (const k of ["status", "priority", "type", "assignee", "label"]) J(`count.by-${k}`, ["count", `--by-${k}`]);
  for (const st of ctx.statuses) J(`count.status.${slug(st)}`, ["count", "--status", st]);

  // --- 4/15: list variants (ui-calls.md args verbatim) ------------------------------------
  J("list.default", ["list"]);
  J("list.all.unlimited", ["list", "--all", "--limit", "0"]);
  J("list.flat", ["list", "--flat"]);
  J("list.brief", ["list", "--brief", "--limit", "0"]);
  J("list.limit5", ["list", "--limit", "5"]);
  J("list.reverse", ["list", "--reverse", "--limit", "20"]);
  J("list.ready-flag", ["list", "--ready"]);
  J("list.no-assignee", ["list", "--no-assignee", "--limit", "0"]);
  J("list.no-labels", ["list", "--no-labels", "--limit", "0"]);
  J("list.no-parent", ["list", "--no-parent", "--limit", "0"]);
  J("list.has-metadata-card_q", ["list", "--has-metadata-key", "card_q", "--limit", "0"]);
  J("list.created-after", ["list", "--created-after", "2026-10-20", "--limit", "0"]);
  J("list.desc-contains", ["list", "--desc-contains", "test", "--limit", "20"]);
  for (const st of BOARD_STATUSES) {
    const lim = st === "closed" ? "1000" : "0";
    J(`list.board.${st}`, ["list", "--tree=false", "--status", st, "--limit", lim]); // bdui-patch-board.mjs:41
  }
  for (const st of ctx.statuses.filter((s) => !BOARD_STATUSES.includes(s)))
    J(`list.status.${slug(st)}`, ["list", "--tree=false", "--status", st, "--limit", "0"]);
  for (const a of ctx.assignees.slice(0, 3)) J(`list.assignee.${slug(a)}`, ["list", "--assignee", a, "--limit", "0"]);
  for (const a of ctx.assignees.slice(0, 1)) J(`list.assignee-status.${slug(a)}`, ["list", "--status", "in_progress", "--assignee", a, "--limit", "0"]);
  for (const l of ctx.labels.slice(0, 2)) J(`list.label.${slug(l)}`, ["list", "--label", l, "--limit", "0"]);
  for (const t of ctx.types.slice(0, 3)) J(`list.type.${slug(t)}`, ["list", "--type", t, "--limit", "0"]);
  for (const p of [0, 1, 2]) J(`list.priority.${p}`, ["list", "--priority", String(p), "--limit", "0"]);
  if (ctx.parent) J("list.parent", ["list", "--parent", ctx.parent, "--limit", "0"]);
  if (ctx.samples.length) J("list.id-filter", ["list", "--id", ctx.samples.slice(0, 5).join(","), "--limit", "0"]);
  T("list.text.default", ["list", "--no-pager"]);
  T("list.text.long", ["list", "--long", "--limit", "10", "--no-pager"]);

  // --- 7/8: ready / blocked ---------------------------------------------------------------
  J("ready", ["ready"]);
  J("ready.limit1000", ["ready", "--limit", "1000"]); // ui-calls.md:43
  J("ready.brief", ["ready", "--brief"]);
  J("ready.explain", ["ready", "--explain"]);
  if (ctx.assignees[0]) J(`ready.assignee.${slug(ctx.assignees[0])}`, ["ready", "--assignee", ctx.assignees[0]]);
  J("blocked", ["blocked"]);
  T("ready.text", ["ready"]);
  T("blocked.text", ["blocked"]);

  // --- 1: show: EVERY id, batched (a single call per id would take hours on upstream) ----
  const ids = ctx.ids;
  for (let i = 0, n = 0; i < ids.length; i += showBatch, n++) {
    J(`show.batch${String(n).padStart(3, "0")}`, ["show", ...ids.slice(i, i + showBatch)]);
  }
  // Single-id shape + flag variants on sampled ids (the UI path: show ID --json --include-dependents)
  for (const id of ctx.samples) {
    J(`show.one.${slug(id)}`, ["show", id]);
    J(`show.dependents.${slug(id)}`, ["show", id, "--include-dependents"]); // list-adapters.js:83
  }
  for (const id of ctx.samples.slice(0, 4)) {
    J(`show.comments.${slug(id)}`, ["show", id, "--include-comments"]);
    J(`show.brief-deps.${slug(id)}`, ["show", id, "--brief-deps"]);
    T(`show.text.${slug(id)}`, ["show", id]);
    T(`show.long.${slug(id)}`, ["show", id, "--long"]);
  }
  T("show.short", ["show", ...ctx.samples.slice(0, 3), "--short"]);

  // --- 3: comments (sampled: one process per id) ------------------------------------------
  for (const id of ctx.commentIds.slice(0, commentsSample)) J(`comments.${slug(id)}`, ["comments", id]);
  for (const id of ctx.samples.slice(0, 3)) T(`comments.text.${slug(id)}`, ["comments", id]);

  // --- 8/9: dep, label --------------------------------------------------------------------
  for (const id of ctx.samples.slice(0, 6)) {
    J(`dep.list.${slug(id)}`, ["dep", "list", id]);
    T(`dep.tree.${slug(id)}`, ["dep", "tree", id]);
    J(`label.list.${slug(id)}`, ["label", "list", id]);
  }
  J("dep.cycles", ["dep", "cycles"]);
  J("label.list-all", ["label", "list-all"]);

  // --- 11: config -------------------------------------------------------------------------
  J("config.list", ["config", "list"]);
  for (const k of ["issue_prefix", "status.custom", "types.custom", "export.auto", "sync.remote", "beads.role"])
    T(`config.get.${slug(k)}`, ["config", "get", k]);

  // --- 10: export (stdout and -o file) ----------------------------------------------------
  m.push({ name: "export.stdout", args: ["export"], kind: "text" });
  m.push({ name: "export.all", args: ["export", "--all"], kind: "text" });
  m.push({ name: "export.file", args: ["export", "-o", "{OUT}/export.file.jsonl"], kind: "file", file: "export.file.jsonl" });

  // --- misc plumbing ----------------------------------------------------------------------
  J("version", ["version"]);
  T("version.text", ["version"]);
  // error shapes: a script that reads these needs the exit code and stderr to match
  m.push({ name: "err.show-missing", args: ["show", "nope-doesnotexist-1", "--json"], kind: "json" });
  m.push({ name: "err.list-bad-status", args: ["list", "--status", "not_a_status", "--json"], kind: "json" });
  m.push({ name: "err.comments-missing", args: ["comments", "nope-doesnotexist-1", "--json"], kind: "json" });
  return m;
}
