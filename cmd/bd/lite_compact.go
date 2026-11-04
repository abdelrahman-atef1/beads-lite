package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/beads/internal/lite/compact"
	"github.com/steveyegge/beads/internal/types"
)

// beads-lite W14: compact text for `show` and `list` (SC-006) and `--fields`
// for --json output. The formatter is internal/lite/compact (T038); these are
// the flags and the small shared helpers the `// lite:` hooks in show.go,
// list.go and output.go call. --full restores upstream text; --json without
// --fields is unchanged.

var (
	liteFull     bool
	liteFields   string
	liteSections []string
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&liteFull, "full", false, "Print upstream's full text output (show/list print a compact form by default)")
	rootCmd.PersistentFlags().StringSliceVar(&liteSections, "section", nil, "show: print the named '## section' in full (substring, repeatable)")
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

// liteCommentsQuery builds the `bd comments` query from its flags and, when
// any is set, prints the result and reports true (beads-lite-1bo5.6.46.5).
func liteCommentsQuery(cmd *cobra.Command, id string, cmts []*types.Comment) (bool, error) {
	var q compact.Query
	q.Titles, _ = cmd.Flags().GetBool("titles")
	q.Last, _ = cmd.Flags().GetInt("last")
	q.N, _ = cmd.Flags().GetInt("n")
	q.Author, _ = cmd.Flags().GetString("author")
	q.Sections = liteSections
	if s, _ := cmd.Flags().GetString("since"); s != "" {
		t, err := compact.ParseSince(s, time.Now())
		if err != nil {
			return true, err
		}
		q.Since = t
	}
	if q.Last < 0 || q.N < 0 {
		return true, fmt.Errorf("--last and --n must be positive")
	}
	if !q.Active() {
		return false, nil
	}
	out, err := compact.Comments(id, cmts, q)
	if err != nil {
		return true, err
	}
	fmt.Print(out)
	return true, nil
}

func addLiteCommentsFlags(c *cobra.Command) {
	c.Flags().Bool("titles", false, "One line per comment: #n date author title")
	c.Flags().Int("last", 0, "Only the N newest comments, folded")
	c.Flags().Int("n", 0, "Print comment #K, exact text")
	c.Flags().String("since", "", "Only comments from YYYY-MM-DD or newer than an age (12h, 3d, 2w)")
	c.Flags().String("author", "", "Only comments by this author")
}
