# Cases: bd folded output and comments query flags (beads-lite-1bo5.6.46.2, .5)

Written blind, from the two beads and the agent-user's point of view (an agent that must read a bead cheaply and
must never lose a verdict or a "Next:" line). Mapping to tests comes in a later commit.

## Show (.2)
- S01 Bead whose description has `## ` sections: default `bd show` prints title, lead, and `[+] Name (N lines)` per section, no section bodies.
- S02 `--section X` prints only section X (substring, case-insensitive); repeatable; no match gives a clear message, not silence.
- S03 `--full` prints everything, identical to upstream (production bd) `--full` output.
- S04 Old flat comment <= 200 chars prints whole.
- S05 Long flat comment: head + marker with line/char counts + tail; the tail holds the verdict / "Next:" / signature.
- S06 Marker hint names the right comment index; running the hint (`bd comments <id> --n K`) prints that comment.
- S07 More than 3 comments: newest 3 shown, "(K older ...)" hint with the right K; indexes (#n) are the real indexes.
- S08 Exactly 3 / 2 / 1 / 0 comments: no stray older-hint, no crash.
- S09 Empty bead (no description, design, notes, comments): clean output, no empty headers, rc 0.
- S10 Arabic title/description/comments: no half-cut character (output valid UTF-8, no U+FFFD), cut counts in runes.
- S11 Very long single comment (500 KB) and 5000-line comment: output bounded, fast (< 2 s), tail still present.
- S12 Section edge cases: `##` without a space, `## ` inside a code fence, duplicate section names, Arabic section name, CRLF line endings, trailing `## ` with empty name.
- S13 Description/design/notes (fields): unchanged from before for flat text (same as production bd).
- S14 `--json` output byte-identical to production bd (with and without `--full`, `--include-comments`).
- S15 Header: status, assignee, blockers/deps, open decision card still printed in default show (never dropped).
- S16 No verdict lost: for flat comments shown, the last 60 runes of the comment appear in output; for sectioned comments, the names of Verdict / Next / Result sections appear (and are reachable with --section).
- S17 Tokens on 20 beads chosen independently: default total below production default total; default <= ~19% of --full; worst bead small.

## Comments flags (.5)
- C01 `bd comments <id>` with no flags: same as production bd.
- C02 `--titles`: one line per comment `#n date author head`, all comments, in order, footer sensible.
- C03 `--last N`: N newest, folded; N=0, N=-1, N=abc, N > count, N huge, N=1.
- C04 `--n K`: exact text of comment K (byte-equal to the text in `--json`); K=0, K=-1, K=count+1, K=abc, K huge; Arabic comment exact.
- C05 `--since`: `YYYY-MM-DD`, `12h`, `3d`, `2w`; bad: empty, `yesterday`, `0d`, `-1d`, `2026-13-45`, `3x`, `d`, far future date, far past date.
- C06 `--author`: exact, case-insensitive; unknown author gives an honest "no comments"; author with partial name does not match; empty value.
- C07 `--section`: only the named section of comments that have it; comments without it are skipped or labelled, not crash.
- C08 Combinations: `--author + --last`, `--since + --titles`, `--n + --titles`, `--n + --last` (conflict handled clearly, not silently).
- C09 Original `#n` numbers are kept after any filter.
- C10 "K hidden: bd comments <id> --titles" footer present when something is hidden, correct K, absent otherwise.
- C11 Every flag on a bead with 11 comments.
- C12 `--json` with flags: untouched (valid JSON, all comments).
- C13 Bead with 0 comments and flags: friendly, rc 0 or clear message.
- C14 Unknown id, moved id: error / moved-id hint as before.

## Regressions
- R01 `bd show`, `bd list`, `bd ready` outputs equal production bd on a copy (list/ready byte-identical).
- R02 `bd comments add <id> "text"` still works on a COPY and the new comment shows with the next index.
- R03 Moved-id hint (`bd show <old-id>` says "moved to ...").
- R04 `bd comments <id>` of a long Arabic comment is unchanged vs production.
- R05 Wall time of `bd show` not slower than production by more than 100 ms on a 1000-bead copy.
