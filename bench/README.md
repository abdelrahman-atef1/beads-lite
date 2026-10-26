# bench

Timing and stress harness (T004). Node only, no deps. Writes go only to a copy: put an empty `.bench-copy` file in the project root.

- `run.mjs` median-of-N per command, optional background agents and board polling.
- `stress.mjs` W writers x M writes (+ optional backup loop), checks failures, lost writes, max call.
- `gen12x.mjs` replicate a `bd export` JSONL 12x for `bd import`.
- `results/` saved runs; `baseline-2026-10.json` = upstream bd 1.3.1 on a copy of the company store.
