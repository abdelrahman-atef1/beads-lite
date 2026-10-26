package sqlitestore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// EnableHistory registers the lite_issue_versions writer (ADR 0001 section 4)
// on the store's before-commit hook list. Call once, right after Open.
func (s *Store) EnableHistory() { s.RegisterBeforeCommit(s.writeVersions) }

// textBundle is the canonical form of the five large columns. Field order is
// fixed so equal content always hashes equal.
type textBundle struct {
	Description        string `json:"description"`
	Design             string `json:"design"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
	Notes              string `json:"notes"`
	Metadata           string `json:"metadata"`
}

// writeVersions snapshots every issue in lite_touched into lite_issue_versions
// (one version per touched issue per transaction) and empties lite_touched.
func (s *Store) writeVersions(ctx context.Context, tx *sql.Tx) error {
	ids, err := touchedIDs(ctx, tx)
	if err != nil || len(ids) == 0 {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range ids {
		scalars, bundle, ok, err := snapshotIssue(ctx, tx, id)
		if err != nil {
			return err
		}
		if !ok { // deleted in this transaction: nothing to snapshot
			continue
		}
		body, _ := json.Marshal(bundle)
		sum := sha256.Sum256(body)
		hash := hex.EncodeToString(sum[:])
		if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO lite_version_text (hash, body) VALUES (?, ?)", hash, string(body)); err != nil {
			return fmt.Errorf("history: text: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO lite_issue_versions (issue_id, committed_at, committer, scalars, text_hash) VALUES (?, ?, ?, ?, ?)",
			id, now, s.actor, string(scalars), hash); err != nil {
			return fmt.Errorf("history: version: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM lite_touched"); err != nil {
		return fmt.Errorf("history: clear touched: %w", err)
	}
	return nil
}

func touchedIDs(ctx context.Context, tx *sql.Tx) ([]string, error) {
	rows, err := tx.QueryContext(ctx, "SELECT issue_id FROM lite_touched ORDER BY issue_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

var largeCols = map[string]bool{"description": true, "design": true, "acceptance_criteria": true, "notes": true, "metadata": true}

// snapshotIssue returns the scalar JSON (every column but the large five,
// NULL and empty values omitted) and the text bundle for one issue row.
func snapshotIssue(ctx context.Context, tx *sql.Tx, id string) ([]byte, textBundle, bool, error) {
	var b textBundle
	rows, err := tx.QueryContext(ctx, "SELECT * FROM issues WHERE id = ?", id)
	if err != nil {
		return nil, b, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, b, false, rows.Err()
	}
	cols, _ := rows.Columns()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, b, false, err
	}
	scalars := map[string]any{}
	for i, c := range cols {
		v := normalize(vals[i])
		if largeCols[c] {
			str, _ := v.(string)
			switch c {
			case "description":
				b.Description = str
			case "design":
				b.Design = str
			case "acceptance_criteria":
				b.AcceptanceCriteria = str
			case "notes":
				b.Notes = str
			case "metadata":
				b.Metadata = str
			}
			continue
		}
		if v == nil || v == "" {
			continue
		}
		scalars[c] = v
	}
	out, err := json.Marshal(scalars) // map keys are sorted: stable bytes
	return out, b, true, err
}

func normalize(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case time.Time:
		return x.UTC().Format(time.RFC3339)
	}
	return v
}
