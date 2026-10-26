package sqlitestore_test

import (
	"flag"
	"os"
	"testing"
)

// knownXFail lists conformance cases that fail today, as a -test.skip regexp.
// It is empty since gate G1 fixes (all 11 earlier cases pass); add a pattern
// with its cause here rather than deleting a failing test. Skipped only when
// the caller did not pass -test.skip; BD_LITE_XFAIL=0 disables the list.
const knownXFail = ``

func TestMain(m *testing.M) {
	if knownXFail != "" && os.Getenv("BD_LITE_XFAIL") != "0" {
		if f := flag.Lookup("test.skip"); f != nil && f.Value.String() == "" {
			_ = f.Value.Set(knownXFail)
		}
	}
	os.Exit(m.Run())
}
