package sqlitestore

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func histStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), dbPath(t), Options{Create: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.EnableHistory()
	return s
}

func insertIssue(ctx context.Context, s *Store, id, status, assignee, notes string) error {
	return s.RunInTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO issues (id,title,description,design,acceptance_criteria,notes,status,assignee,metadata)
			VALUES (?,?,?,?,?,?,?,?,?)`, id, "t "+id, "d", "", "", notes, status, assignee, `{"asked_at":"2026-10-25T09:00:00Z"}`)
		return err
	})
}

func TestOneVersionPerCommandNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := histStore(t)
	s.actor = "swe"
	if err := insertIssue(ctx, s, "r-1", "open", "", "n"); err != nil {
		t.Fatal(err)
	}
	// One command touching the row twice still yields one version.
	if err := s.RunInTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE issues SET status='owner_decision', assignee='abdelrahman' WHERE id='r-1'"); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE issues SET priority=1 WHERE id='r-1'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	h, err := s.History(ctx, "r-1", 0)
	if err != nil || len(h) != 2 {
		t.Fatalf("len=%d err=%v", len(h), err)
	}
	if h[0].Issue.Status != "owner_decision" || h[0].Issue.Assignee != "abdelrahman" || h[0].Issue.Priority != 1 {
		t.Fatalf("newest: %+v", h[0].Issue)
	}
	if h[1].Issue.Status != "open" || h[1].Issue.Assignee != "" {
		t.Fatalf("oldest: %+v", h[1].Issue)
	}
	if h[0].Committer != "swe" || h[0].CommitDate.IsZero() || h[0].CommitHash == h[1].CommitHash {
		t.Fatalf("entry meta: %+v", h[0])
	}
	if string(h[0].Issue.Metadata) != `{"asked_at":"2026-10-25T09:00:00Z"}` || h[0].Issue.Description != "d" || h[0].Issue.CreatedAt.IsZero() {
		t.Fatalf("rebuilt fields: %+v", h[0].Issue)
	}
	if l, _ := s.History(ctx, "r-1", 1); len(l) != 1 || l[0].CommitHash != h[0].CommitHash {
		t.Fatalf("limit: %v", l)
	}
	var n int
	_ = s.DB().QueryRow("SELECT count(*) FROM lite_touched").Scan(&n)
	if n != 0 {
		t.Fatalf("lite_touched not emptied: %d", n)
	}
}

func TestNoVersionOnRollbackAndUnknownID(t *testing.T) {
	ctx := context.Background()
	s := histStore(t)
	err := s.RunInTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO issues (id,title,description,design,acceptance_criteria,notes) VALUES ('r-2','x','','','','')"); err != nil {
			return err
		}
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("want error")
	}
	var n int
	_ = s.DB().QueryRow("SELECT count(*) FROM lite_issue_versions").Scan(&n)
	if n != 0 {
		t.Fatalf("versions after rollback: %d", n)
	}
	h, err := s.History(ctx, "nope", 0)
	if err != nil || h == nil || len(h) != 0 {
		t.Fatalf("unknown id: %v %v", h, err)
	}
}

func TestTextDedupe(t *testing.T) {
	ctx := context.Background()
	s := histStore(t)
	if err := insertIssue(ctx, s, "r-3", "open", "", "same"); err != nil {
		t.Fatal(err)
	}
	for _, st := range []string{"in_progress", "blocked"} { // scalar-only changes
		if err := s.RunInTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE issues SET status=? WHERE id='r-3'", st)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	var v, b int
	_ = s.DB().QueryRow("SELECT count(*) FROM lite_issue_versions").Scan(&v)
	_ = s.DB().QueryRow("SELECT count(*) FROM lite_version_text").Scan(&b)
	if v != 3 || b != 1 {
		t.Fatalf("versions=%d bundles=%d", v, b)
	}
	if err := s.RunInTx(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE issues SET notes='changed' WHERE id='r-3'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = s.DB().QueryRow("SELECT count(*) FROM lite_version_text").Scan(&b)
	if b != 2 {
		t.Fatalf("bundles=%d", b)
	}
}

// The rayeh gate (askedSinceFromHistory) reads CommitDate plus
// Issue.assignee and Issue.status; walk back newest first while the run holds.
func TestGateFieldsAndEvents(t *testing.T) {
	ctx := context.Background()
	s := histStore(t)
	_ = insertIssue(ctx, s, "r-4", "open", "swe", "")
	for i := 0; i < 2; i++ {
		_ = s.RunInTx(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE issues SET status='owner_decision', assignee='abdelrahman', priority=? WHERE id='r-4'", i)
			return err
		})
	}
	h, _ := s.History(ctx, "r-4", 0)
	var since string
	run := 0
	for _, e := range h {
		if e.Issue.Assignee != "abdelrahman" || e.Issue.Status != "owner_decision" {
			break
		}
		run++
		since = e.CommitDate.Format("2006-01-02T15:04:05Z")
	}
	if run != 2 || len(since) != 20 {
		t.Fatalf("run=%d since=%q", run, since)
	}
	if _, err := s.DB().Exec("INSERT INTO events (id,issue_id,event_type,actor,comment) VALUES ('e1','r-4','commented','swe','hi'),('e2','r-4','commented','swe','yo')"); err != nil {
		t.Fatal(err)
	}
	it, err := s.IterEvents(ctx, "r-4", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	n := 0
	for it.Next(ctx) {
		n++
	}
	if n != 1 {
		t.Fatalf("events=%d", n)
	}
}
