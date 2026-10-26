# T008 state (swe/t008)
Done: embeddeddolt role files ported to internal/storage/sqlitestore (sed copy, withConn -> RunInTx in plumbing.go), Dolt-only roles in unsupported.go (+allowlist), wire.go (Adapt, SetDoltSQL(false), dialect openDB, RecomputeAllBlocked, regexp()), conformance_test.go (RunAll + RunUnsupportedContract), xfail_test.go (list with causes).
Merged: swe/t007, swe/w1-dialect (73ff1355a). go.sum: 2 hashes added (T006 owns).
Pending at merge: delete history_stub.go when T009's history.go lands (duplicate History).
XFAIL causes (see xfail_test.go): MAX(created_at) scan into time.Time (T005), time compare CreatedBefore/StartedAfter (T005/T006), REGEXP udf not seen by dialect zero-value Driver (T005), lite_touched UNIQUE on CreateRejectStaleUpserts (T005/T006).
Run: go test ./internal/storage/sqlitestore/ (CGO_ENABLED=0, ~7 min loaded); BD_LITE_XFAIL=0 shows the xfails.
