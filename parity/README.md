# Parity oracle and bench harness (P0.2)

Scripts only. The corpus (copies of the real project stores and every saved output) holds
real bead data and lives OUTSIDE this repo, default `E:/Projects/beads-lite-corpus/`
(override with `PARITY_CORPUS` or `--corpus`). Nothing under that path may be committed.

Corpus layout:

    stores/<project>/.beads          working copies (bd may write to them)
    stores-pristine/<project>/.beads untouched snapshot; scratch runs copy from here
    oracle/<label>/<project>/        <command>.out = raw stdout, _index.jsonl = args/exit/ms/sha
    oracle/<label>/writes-<project>/ write-path oracle (scripted writes on a scratch copy)
    oracle/diff-<a>-vs-<b>.json      compare report
    bench/                           timing + stress results
    synthetic/, stores-12x/          the 12x set

Rules: never run bd against a live store (BEADS_DIR is pinned to a copy by `lib.mjs`);
child processes are hidden and killed by PID only; `BD_DISABLE_METRICS=1` always.

## Produce the oracle (upstream bd 1.3.1) and the fork side

    node parity/capture.mjs                                   # all five stores, label upstream-1.3.1
    node parity/writes.mjs --project whisper-flow             # write-path oracle
    BD_BIN=<fork bd.exe> node parity/capture.mjs --label fork # on a MIGRATED copy of the stores
    node parity/compare.mjs --a upstream-1.3.1 --b fork [--strip-time] [--verbose]

Defaults to the real `bd.exe` under `%APPDATA%\npm\node_modules\@beads\bd\bin` (the npm `bd`
is a .cmd shim that `spawn` cannot run). Capture is resumable (`_index.jsonl`); `--force` redoes.
`show` for EVERY id is captured in batches of 100 ids per process (one process per id would
take hours on upstream); single-id shape and flag variants are captured for 12 sampled ids.
`comments <id>` is captured for the 40 beads with most comments.

## Bench (plan section 4)

    node bench/timing.mjs --project company --n 10 --load 5     # table vs targets
    node bench/stress.mjs --writers 20 --writes 50              # 0 locked / 0 lost
    node bench/synth12x.mjs --import                            # build 19,428-bead store, then
    node bench/timing.mjs --store <corpus>/stores-12x/<label> --label ...

Timings are whole-process wall time on THIS machine; other agents' load inflates them.
Record the machine load next to any number you quote.
