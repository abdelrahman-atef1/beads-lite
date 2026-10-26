package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/steveyegge/beads/internal/lite/compact"
)

// beads-lite W14: compact text for `show` and `list` (SC-006) and `--fields`
// for --json output. The formatter is internal/lite/compact (T038); these are
// the flags and the small shared helpers the `// lite:` hooks in show.go,
// list.go and output.go call. --full restores upstream text; --json without
// --fields is unchanged.

var (
	liteFull   bool
	liteFields string
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&liteFull, "full", false, "Print upstream's full text output (show/list print a compact form by default)")
	rootCmd.PersistentFlags().StringVar(&liteFields, "fields", "", "With --json: keep only these top-level fields (comma-separated)")
}

// liteCompactText reports whether show/list should print the compact form:
// not asked for the full text, and not asked for a detailed (--long) layout.
func liteCompactText(long bool) bool {
	return !liteFull && !long
}

// liteSelectFields applies --fields to a value about to be written as JSON.
// On any trouble it returns v untouched: a filter must never break output.
func liteSelectFields(v interface{}) interface{} {
	fields := compact.ParseFields(liteFields)
	if len(fields) == 0 {
		return v
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	sel, err := compact.SelectFields(raw, fields)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: --fields ignored: %v\n", err)
		return v
	}
	return json.RawMessage(sel)
}
