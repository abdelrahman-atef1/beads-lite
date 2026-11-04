// QA run for beads-lite-1bo5.6.46.2 (bd show fold) and .5 (bd comments flags).
// Env: BD_NEW (exe under test), BD_OLD (production bd, baseline), QA_ENV (db copies folder). Writes only to copies.
import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { nw, ol, NEW, freshFixture, create, addComment, jsonComments, validUtf8 } from "./lib.mjs";

const T = mkdtempSync(path.join(tmpdir(), "qa-bdfold-"));
freshFixture("beads-lite");
const ids = {};
const note = (s) => console.log("# " + s);
const nums = (s) => [...s.matchAll(/^#(\d+) /gm)].map((m) => +m[1]);
const nopanic = (r, what) => assert.ok(!/panic|goroutine/.test(r.out + r.err), what + " panics: " + (r.out + r.err).slice(0, 200));

// ---------- fixtures (on the copy) ----------
const flatShort = "Short flat note. Next: nothing.";
const flatLong = (n, tail) => `Review of item ${n}. ` + "word ".repeat(120) + `\nsecond line ${n}\nthird line ${n}\n` + tail;
ids.empty = create("empty bead");
ids.eleven = create("eleven comments bead", { desc: "Lead line of the description.\n\n## Evidence\nline a\nline b\n\n## Verdict\nPASS everything.", assignee: "swe" });
const authors = ["swe", "QA", "cto", "swe", "qa", "pm", "swe", "qa", "cto", "sre", "qa"];
authors.forEach((a, i) => {
  const k = i + 1;
  let t;
  if (k === 3) t = "Ruling.\n## Policy\nA\nB\n## Verdict\nFAIL on item three\n## Next\nswe fixes item three, then qa re-runs";
  else if (k % 2 === 0) t = flatShort + ` #${k}`;
  else t = flatLong(k, `VERDICT-${k}: FAIL because x. Next: swe fixes it. -- ${a}`);
  addComment(ids.eleven, t, a, T);
});
ids.ar = create("بيد: عنوان عربي للاختبار", { desc: "فقرة افتتاحية عربية.\n\n## الأدلة\nسطر ١\nسطر ٢\n\n## الحكم\nنجح" });
const ar = "هذا تعليق عربي طويل جدا للاختبار ".repeat(15) + "\nالحكم: فشل لأن الزر لا يعمل. التالي: المطور يصلح الزر ثم يعيد الاختبار. — عفني (QA)";
addComment(ids.ar, ar, "عفني", T);
addComment(ids.ar, "قصير", "qa", T);
ids.big = create("long comment bead");
addComment(ids.big, "BEGIN " + "x".repeat(500_000) + " VERDICT-TAIL-MARK FAIL. Next: swe fixes. — qa", "qa", T);
addComment(ids.big, Array.from({ length: 5000 }, (_, i) => `line ${i}`).join("\n") + "\nLAST-LINE-VERDICT PASS. Next: none.", "qa", T);
ids.edge = create("section edge cases", {
  desc: "Lead.\n\n##nospace\nnot a section\n\n```\n## in fence\nfoo\n```\n\n## Evidence\none\n\n## Evidence\ntwo\n\n## \nempty name body",
  design: "d1\r\n## الأدلة\r\nx\r\n",
});
addComment(ids.edge, "line1\r\n## Result\r\nPASS\r\n## Next\r\ndo it\r\n", "qa", T);
addComment(ids.edge, "##nospace tail\nverdict PASS. Next: none", "qa", T);

const realIds = (n) => JSON.parse(nw(["list", "--limit", String(n), "--json", "--all"]).out).map((x) => x.id);

// ---------- S: show ----------
test("S01 sectioned description: titles only", () => {
  const o = nw(["show", ids.eleven]).out;
  assert.match(o, /\[\+\] Evidence \(\d+ lines?\)/);
  assert.match(o, /\[\+\] Verdict/);
  assert.ok(!o.includes("line a"), "section body leaked");
  assert.ok(o.includes("Lead line of the description."));
});
test("S02 --section substring, case-insens, repeatable, no match message", () => {
  let o = nw(["show", ids.eleven, "--section", "evid"]).out;
  assert.ok(o.includes("line a") && o.includes("line b"));
  o = nw(["show", ids.eleven, "--section", "Evidence", "--section", "verdict"]).out;
  assert.ok(o.includes("line a") && o.includes("PASS everything."));
  const r = nw(["show", ids.eleven, "--section", "zzzz-nope"]);
  assert.ok(/no section|not found|no match|zzzz-nope/i.test(r.out + r.err), "silent on no match: " + (r.out + r.err).slice(0, 300));
});
test("S03 --full equals production --full (fixtures + 40 real beads)", () => {
  for (const id of [...Object.values(ids), ...realIds(40)]) {
    assert.equal(nw(["show", id, "--full"]).out, ol(["show", id, "--full"]).out, `--full differs for ${id}`);
  }
});
test("S04/S08 short comment whole; 1..3 comments no stray older hint; 4 -> older hint", () => {
  const one = create("one comment"); addComment(one, flatShort, "qa", T);
  const o1 = nw(["show", one]).out; assert.ok(o1.includes(flatShort)); assert.ok(!/older/.test(o1));
  for (let i = 0; i < 2; i++) addComment(one, "more " + i, "qa", T);
  const o3 = nw(["show", one]).out; assert.ok(!/older/.test(o3), "3 comments must show no older hint");
  addComment(one, "four", "qa", T);
  const o4 = nw(["show", one]).out; assert.match(o4, /\(1 older/);
  assert.match(o4, /#2 /); assert.ok(!/#1 /.test(o4));
});
test("S05/S06/S07 long flat folded: tail holds verdict, hint index works, newest 3, real indexes", () => {
  const o = nw(["show", ids.eleven]).out;
  assert.match(o, /\(8 older, bd comments \S+ --titles\)/);
  for (const k of [9, 10, 11]) assert.ok(new RegExp(`- #${k} `).test(o), "label #" + k);
  assert.ok(!/- #8 /.test(o));
  assert.ok(o.includes("VERDICT-9: FAIL because x. Next: swe fixes it. -- cto"), "tail of #9 lost");
  assert.ok(o.includes("VERDICT-11: FAIL because x. Next: swe fixes it. -- qa"), "tail of #11 lost");
  const hint = o.match(/bd comments (\S+) --n (\d+)\]/); assert.ok(hint, "marker with --n hint");
  const full = nw(["comments", hint[1], "--n", hint[2]]).out;
  assert.ok(full.includes("VERDICT-" + hint[2]), "hint #" + hint[2] + " prints other comment");
});
test("S09 empty bead clean and same as production", () => {
  const r = nw(["show", ids.empty]); assert.equal(r.rc, 0);
  assert.ok(!/DESCRIPTION|COMMENTS/.test(r.out), "empty headers: " + r.out);
  assert.equal(r.out, ol(["show", ids.empty]).out);
});
test("S10 Arabic: valid UTF-8, no replacement char, tail present", () => {
  for (const args of [["show", ids.ar], ["comments", ids.ar], ["comments", ids.ar, "--titles"], ["comments", ids.ar, "--last", "1"], ["comments", ids.ar, "--n", "1"], ["show", ids.ar, "--section", "الأدلة"]]) {
    const r = nw(args); assert.ok(validUtf8(r.buf), "invalid utf8 " + args.join(" ")); assert.ok(!r.out.includes("�"), "U+FFFD " + args.join(" "));
  }
  const o = nw(["show", ids.ar]).out;
  assert.ok(o.includes("عفني (QA)"), "Arabic signature lost in tail"); assert.ok(o.includes("المطور يصلح الزر"), "Arabic Next lost");
  assert.ok(nw(["show", ids.ar, "--section", "الأدلة"]).out.includes("سطر ١"));
});
test("S11 huge comments bounded and fast, tail kept", () => {
  const r = nw(["show", ids.big]); assert.ok(r.ms < 2000, "ms " + r.ms); assert.ok(r.out.length < 4000, "len " + r.out.length);
  assert.ok(r.out.includes("VERDICT-TAIL-MARK FAIL. Next: swe fixes. — qa"), "500KB tail lost");
  assert.ok(r.out.includes("LAST-LINE-VERDICT PASS. Next: none."), "5000-line tail lost");
});
test("S12 section edge cases don't crash / lose text", () => {
  const r = nw(["show", ids.edge]); assert.equal(r.rc, 0, r.err);
  assert.ok(validUtf8(r.buf));
  const sec = nw(["show", ids.edge, "--section", "Evidence"]).out;
  assert.ok(sec.includes("one") && sec.includes("two"), "duplicate Evidence sections: both should print:\n" + sec);
  const fence = r.out.match(/\[\+\][^\n]*fence[^\n]*/);
  assert.ok(!fence, "'## in fence' inside code fence treated as a section: " + (fence && fence[0]));
  assert.equal(nw(["show", ids.edge, "--section", "Result"]).rc, 0);
  note("edge show output: " + JSON.stringify(r.out.slice(0, 700)));
});
test("S13 flat fields: every base line is a prefix of a new line (nothing reordered/dropped; only less cut), max 15 lines/field", () => {
  const bad = [];
  for (const id of realIds(60)) {
    const head = (o) => o.split("\nCOMMENTS")[0].split("\n");
    const nH = head(nw(["show", id]).out), bH = head(ol(["show", id]).out);
    if (bH.some((l) => /^## /.test(l))) continue;
    for (const l of bH) if (l && !nH.some((x) => x.startsWith(l))) { bad.push(id + ": " + l.slice(0, 50)); break; }
  }
  assert.deepEqual(bad, []);
});
test("S14 show --json byte-identical to production", () => {
  for (const id of [...Object.values(ids), ...realIds(30)]) for (const extra of [[], ["--full"], ["--include-comments"], ["--full", "--include-comments"]]) {
    assert.equal(nw(["show", id, "--json", ...extra]).out, ol(["show", id, "--json", ...extra]).out, `json differs ${id} ${extra}`);
  }
  for (const id of Object.values(ids)) assert.equal(nw(["comments", id, "--json"]).out, ol(["comments", id, "--json"]).out);
});
test("S15 header lines (status/assignee/deps/card) equal production on 200 beads", () => {
  const bad = [];
  const h = (s) => s.split(/\n(?:DESCRIPTION|DESIGN|NOTES|COMMENTS|ACCEPTANCE)/)[0].split("\n").filter((l) => !/^(created|updated)/.test(l)).join("\n");
  for (const id of realIds(200)) if (h(nw(["show", id]).out) !== h(ol(["show", id]).out)) bad.push(id);
  assert.deepEqual(bad, []);
});
test("S16 verdict / Next of a SECTIONED comment inside the newest 3 is visible in default show", () => {
  const e = create("sectioned in window"); addComment(e, "x", "qa", T);
  addComment(e, "Ruling.\n## Policy\nA\n## Verdict\nFAIL on item three\n## Next\nswe fixes item three, then qa re-runs", "cto", T);
  const s = nw(["show", e]).out;
  note("sectioned comment shown as: " + JSON.stringify(s.split("COMMENTS")[1]));
  assert.ok(s.includes("FAIL on item three") && s.includes("swe fixes item three"), "Verdict/Next text of a sectioned comment is NOT in default show (only [+] titles)");
});

// ---------- C: comments flags ----------
const body = (o) => o.split("
").slice(1).join("
").trim();
const cmt = (...a) => nw(["comments", ids.eleven, ...a]);
test("C01 no flags same as production", () => {
  for (const id of Object.values(ids)) assert.equal(nw(["comments", id]).out, ol(["comments", id]).out, id);
});
test("C02 --titles one line per comment", () => {
  const r = cmt("--titles"); assert.equal(r.rc, 0, r.err);
  const lines = r.out.split("\n").filter((l) => /^#\d+ /.test(l)); assert.equal(lines.length, 11, r.out);
  assert.match(lines[0], /^#1 2026-\d\d-\d\d\S* swe /);
  assert.ok(lines.every((l) => [...l].length < 140), "long titles line");
  note("titles output: " + JSON.stringify(r.out));
});
test("C03 --last N", () => {
  let r = cmt("--last", "2"); assert.equal(r.rc, 0); assert.ok(/#10 /.test(r.out) && /#11 /.test(r.out) && !/#9 /.test(r.out), r.out);
  assert.match(r.out, /9 hidden: bd comments \S+ --titles/);
  r = cmt("--last", "1"); assert.ok(/#11 /.test(r.out) && !/#10 /.test(r.out));
  r = cmt("--last", "99"); assert.equal(r.rc, 0); assert.ok(/#1 /.test(r.out) && !/hidden/.test(r.out));
  for (const bad of ["0", "-1", "abc", "1.5", "99999999999999999999", ""]) {
    r = cmt("--last", bad); nopanic(r, "--last " + bad);
    note(`--last '${bad}' -> rc ${r.rc}, ${JSON.stringify((r.out + r.err).trim().slice(0, 90))}`);
    assert.ok(r.rc !== 0 || /\S/.test(r.out + r.err), `--last ${bad}: silent`);
  }
});
test("C04 --n K exact text; bad K fails loudly", () => {
  const js = jsonComments(NEW, ids.eleven);
  for (let k = 1; k <= 11; k++) {
    const r = cmt("--n", String(k)); assert.equal(r.rc, 0, r.err);
    assert.match(r.out.split("
")[0], new RegExp("^#" + k + " \d{4}-\d\d-\d\d \S+:$"));
    assert.equal(body(r.out), js[k - 1].text.trim(), "#" + k + " text differs");
  }
  const a = jsonComments(NEW, ids.ar); assert.equal(body(nw(["comments", ids.ar, "--n", "1"]).out), a[0].text.trim(), "Arabic #1 not exact");
  const b = jsonComments(NEW, ids.big);
  assert.equal(body(nw(["comments", ids.big, "--n", "1"]).out), b[0].text.trim(), "500KB comment not exact");
  assert.equal(body(nw(["comments", ids.big, "--n", "2"]).out), b[1].text.trim(), "5000-line comment not exact");
  for (const bad of ["0", "-1", "12", "abc", "99999999999999999999", ""]) {
    const x = cmt("--n", bad); nopanic(x, "--n " + bad);
    note(`--n '${bad}' -> rc ${x.rc}, ${JSON.stringify((x.out + x.err).trim().slice(0, 90))}`);
    assert.notEqual(x.rc, 0, `--n ${bad} should fail`);
    assert.ok(/\S/.test(x.out + x.err), `--n ${bad}: silent failure`);
  }
});
test("C05 --since valid and invalid", () => {
  for (const ok of ["2026-01-01", "12h", "3d", "2w", "1d", "100000d"]) { const r = cmt("--since", ok, "--titles"); assert.equal(r.rc, 0, ok + r.err); }
  assert.equal(nums(cmt("--since", "2099-01-01", "--titles").out).length, 0, "future date returns comments");
  assert.equal(nums(cmt("--since", "2020-01-01", "--titles").out).length, 11);
  const loud = [];
  for (const b of ["", "yesterday", "0d", "-1d", "2026-13-45", "3x", "d", "2026-02-30", "1.5d", "٣d", "5"]) {
    const r = cmt("--since", b, "--titles"); nopanic(r, "--since " + b);
    note(`--since '${b}' -> rc ${r.rc}, ${nums(r.out).length} lines, ${JSON.stringify((r.out + r.err).trim().slice(0, 80))}`);
    if (r.rc === 0 && nums(r.out).length > 0 && b !== "1.5d") loud.push(b);
  }
  assert.deepEqual(loud, [], "bad --since values accepted silently (returned comments, rc 0)");
});
test("C06 --author exact, case-insensitive, unknown honest", () => {
  assert.deepEqual(nums(cmt("--author", "qa", "--titles").out), [2, 5, 8, 11]);
  assert.deepEqual(nums(cmt("--author", "QA", "--titles").out), [2, 5, 8, 11]);
  assert.deepEqual(nums(cmt("--author", "sw", "--titles").out), [], "partial author matched");
  let r = cmt("--author", "nobody", "--titles"); assert.ok(/no comments|none|0 comments|nobody|no match/i.test(r.out + r.err), "silent for unknown author: " + JSON.stringify(r.out + r.err));
  r = cmt("--author", "", "--titles"); nopanic(r, "--author ''"); note(`--author '' -> rc ${r.rc}, ${nums(r.out).length} lines`);
  assert.equal(nums(nw(["comments", ids.ar, "--author", "عفني", "--titles"]).out).length, 1);
});
test("C07 --section on comments", () => {
  const r = cmt("--section", "Verdict"); assert.equal(r.rc, 0, r.err); assert.ok(r.out.includes("FAIL on item three"), r.out);
  const r2 = cmt("--section", "nope-zz"); nopanic(r2, "--section nope"); assert.ok(/\S/.test(r2.out + r2.err), "silent");
  note("--section Verdict output: " + JSON.stringify(r.out.slice(0, 400)));
});
test("C08/C09 combinations keep original numbers; conflicts are explicit", () => {
  const r = cmt("--author", "qa", "--last", "2"); assert.ok(/#8 /.test(r.out) && /#11 /.test(r.out) && !/#5 /.test(r.out), r.out);
  assert.equal(nums(cmt("--since", "1d", "--titles").out).length, 11);
  const c1 = cmt("--n", "3", "--titles"), c2 = cmt("--n", "3", "--last", "2");
  note(`--n 3 --titles -> rc ${c1.rc}: ${JSON.stringify(c1.out.slice(0, 80))}; --n 3 --last 2 -> rc ${c2.rc}: ${JSON.stringify(c2.out.slice(0, 80))}`);
  assert.ok(c1.rc !== 0 || /\S/.test(c1.out)); assert.ok(c2.rc !== 0 || /\S/.test(c2.out));
});
test("C10 hidden footer only when hidden, right K", () => {
  const r = cmt("--titles"); note("titles footer: " + JSON.stringify(r.out.trim().split("\n").slice(-2)));
  assert.match(cmt("--last", "3").out, /8 hidden: bd comments \S+ --titles/);
  const one = create("one"); addComment(one, "a", "qa", T);
  assert.ok(!/hidden/.test(nw(["comments", one, "--titles"]).out));
});
test("C12 --json with flags untouched (all 11, valid json)", () => {
  for (const f of [["--titles"], ["--last", "2"], ["--n", "2"], ["--author", "qa"], ["--since", "1d"]]) {
    const r = cmt("--json", ...f); let j; try { j = JSON.parse(r.out); } catch { assert.fail(`--json ${f}: not json: ${r.out.slice(0, 100)} ${r.err}`); }
    assert.equal(Array.isArray(j) ? j.length : 1, 11, `--json ${f} filtered`);
  }
});
test("C13 zero comments with flags: no panic, honest", () => {
  for (const f of [[], ["--titles"], ["--last", "2"], ["--author", "x"], ["--since", "1d"], ["--n", "1"]]) {
    const r = nw(["comments", ids.empty, ...f]); nopanic(r, f.join(" "));
    note(`empty ${f.join(" ")} -> rc ${r.rc}: ${JSON.stringify((r.out + r.err).trim().slice(0, 80))}`);
  }
});
test("C14 unknown id; moved id hint same as production", () => {
  const a = nw(["comments", "beads-lite-zzzz", "--titles"]);
  assert.notEqual(a.rc, 0); assert.ok(/\S/.test(a.out + a.err));
  const m = nw(["show", "company-8ce8.14.10"]), mo = ol(["show", "company-8ce8.14.10"]);
  assert.equal((m.out + m.err).trim(), (mo.out + mo.err).trim(), "moved-id output differs from production");
  note("moved-id: " + JSON.stringify((m.out + m.err).trim().slice(0, 160)));
});

// ---------- R: regressions ----------
test("R01 list/ready outputs equal production", () => {
  for (const a of [["list"], ["list", "--status", "closed", "--limit", "50"], ["ready"], ["list", "--json"], ["ready", "--json"], ["list", "--brief"]]) {
    const x = nw(a), y = ol(a); assert.equal(x.rc, y.rc, a.join(" ")); assert.equal(x.out, y.out, "differs: " + a.join(" "));
  }
});
test("R02 comments add on copy, next index", () => {
  const e = create("add test"); addComment(e, "first", "qa", T); addComment(e, "second", "qa", T);
  assert.match(nw(["comments", e, "--titles"]).out, /#2 /);
  const r2 = nw(["comments", "add", e, "inline text arg"]); assert.equal(r2.rc, 0, r2.err);
  assert.match(nw(["show", e]).out, /#3 .*qa:\n\s+inline text arg/);
});
test("R05 timing vs production", () => {
  const t = (f) => { const t0 = Date.now(); for (let i = 0; i < 5; i++) f(); return (Date.now() - t0) / 5; };
  const n = t(() => nw(["show", ids.eleven])), o = t(() => ol(["show", ids.eleven]));
  note(`show ms new ${n} old ${o}`); assert.ok(n < o + 150, `new ${n} vs old ${o}`);
});
