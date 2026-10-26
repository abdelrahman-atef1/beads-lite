package sqlitestore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/steveyegge/beads/internal/config"
	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/storage/issueops"
	"github.com/steveyegge/beads/internal/types"
	"github.com/steveyegge/beads/internal/utils"
)

// GetIssue is implemented in get_issue.go.

func (s *Store) GetIssueByExternalRef(ctx context.Context, externalRef string) (*types.Issue, error) {
	var id string
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		id, err = issueops.GetIssueByExternalRefInTx(ctx, tx, externalRef)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetIssue(ctx, id)
}

// GetIssuesByIDs is implemented in dependencies.go.

// UpdateIssue is implemented in issues.go.

// CloseIssue is implemented in issues.go.

func (s *Store) DeleteIssue(ctx context.Context, id string) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		// storage.DeleteIssue carries no actor, so the journal rows record none.
		return issueops.DeleteIssueInTx(ctx, tx, id, "")
	})
}

// AddDependency is implemented in dependencies.go.

// RemoveDependency is implemented in dependencies.go.

func (s *Store) GetDependencies(ctx context.Context, issueID string) ([]*types.Issue, error) {
	var result []*types.Issue
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetDependenciesInTx(ctx, tx, issueID)
		return err
	})
	return result, err
}

func (s *Store) GetDependents(ctx context.Context, issueID string) ([]*types.Issue, error) {
	var result []*types.Issue
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetDependentsInTx(ctx, tx, issueID)
		return err
	})
	return result, err
}

// GetDependenciesWithMetadata is implemented in dependencies.go.

// GetDependentsWithMetadata is implemented in dependencies.go.

func (s *Store) GetDependencyTree(ctx context.Context, issueID string, maxDepth int, showAllPaths bool, reverse bool) ([]*types.TreeNode, error) {
	var result []*types.TreeNode
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetDependencyTreeInTx(ctx, tx, issueID, maxDepth, showAllPaths, reverse)
		return err
	})
	return result, err
}

// AddLabel is implemented in labels.go.

// RemoveLabel is implemented in labels.go.

// GetLabels is implemented in labels.go.

func (s *Store) GetIssuesByLabel(ctx context.Context, label string) ([]*types.Issue, error) {
	var ids []string
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		ids, err = issueops.GetIssuesByLabelInTx(ctx, tx, label)
		return err
	})
	if err != nil {
		return nil, err
	}
	return s.GetIssuesByIDs(ctx, ids)
}

// GetReadyWork is implemented in queries.go.

func (s *Store) GetBlockedIssues(ctx context.Context, filter types.WorkFilter) ([]*types.BlockedIssue, error) {
	var result []*types.BlockedIssue
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetBlockedIssuesInTx(ctx, tx, filter)
		return err
	})
	return result, err
}

func (s *Store) GetEpicsEligibleForClosure(ctx context.Context) ([]*types.EpicStatus, error) {
	var result []*types.EpicStatus
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetEpicsEligibleForClosureInTx(ctx, tx)
		return err
	})
	return result, err
}

func (s *Store) AddIssueComment(ctx context.Context, issueID, author, text string) (*types.Comment, error) {
	var result *types.Comment
	err := s.withConn(ctx, true, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.AddIssueCommentInTx(ctx, tx, issueID, author, text)
		return err
	})
	return result, err
}

func (s *Store) GetIssueComments(ctx context.Context, issueID string) ([]*types.Comment, error) {
	var result []*types.Comment
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetIssueCommentsInTx(ctx, tx, issueID)
		return err
	})
	return result, err
}

// GetIssueCommentsPage returns one keyset page of an issue's comments in
// (created_at ASC, id ASC) order, resuming strictly after the cursor. See the
// storage.Storage doc for the ordering, sargability, and page-walk-equals-full-
// read contract.
func (s *Store) GetIssueCommentsPage(ctx context.Context, issueID string, after storage.CommentPageCursor, limit int) ([]*types.Comment, error) {
	var result []*types.Comment
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetIssueCommentsPageInTx(ctx, tx, issueID, after, limit)
		return err
	})
	return result, err
}

func (s *Store) GetEvents(ctx context.Context, issueID string, limit int) ([]*types.Event, error) {
	var result []*types.Event
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetEventsInTx(ctx, tx, issueID, limit)
		return err
	})
	return result, err
}

func (s *Store) GetAllEventsSince(ctx context.Context, since time.Time) ([]*types.Event, error) {
	var result []*types.Event
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetAllEventsSinceInTx(ctx, tx, since)
		return err
	})
	return result, err
}

// EventsSince returns durable events strictly after the keyset cursor, ordered
// by (created_at ASC, id ASC) and bounded by limit. Durable events table only.
// issueID != "" scopes the feed to one bead's history.
func (s *Store) EventsSince(ctx context.Context, cursor storage.EventCursor, issueID string, limit int) ([]*types.Event, error) {
	var result []*types.Event
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.EventsSinceInTx(ctx, tx, cursor.CreatedAt, cursor.ID, issueID, limit)
		return err
	})
	return result, err
}

// RecordProvenanceEvent appends a provenance event idempotently. inserted is
// false when the deterministic id already existed. Append-only — no update path.
func (s *Store) RecordProvenanceEvent(ctx context.Context, ev types.ProvenanceEvent) (id string, inserted bool, err error) {
	err = s.withConn(ctx, true, func(tx *sql.Tx) error {
		var txErr error
		id, inserted, txErr = issueops.RecordProvenanceEventInTx(ctx, tx, ev)
		return txErr
	})
	if err != nil {
		return "", false, err
	}
	return id, inserted, nil
}

func (s *Store) GetProvenanceEvents(ctx context.Context, issueID, kindFilter string) ([]types.ProvenanceEvent, error) {
	var result []types.ProvenanceEvent
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetProvenanceEventsInTx(ctx, tx, issueID, kindFilter)
		return err
	})
	return result, err
}

func (s *Store) GetProvenanceByRef(ctx context.Context, ref string) ([]types.ProvenanceEvent, error) {
	var result []types.ProvenanceEvent
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetProvenanceByRefInTx(ctx, tx, ref)
		return err
	})
	return result, err
}

// RunInTransaction is implemented in transaction.go.

// Close decrements the reference count if this store was opened via Open (the
// process-scoped cache). When other references remain, Close is a no-op — the
// store stays alive for the remaining callers. When the last reference calls
// Close (or if the store was created directly via newStore), the underlying
// resources are released.
//
// It is safe to call multiple times.
// DoltGC runs Dolt's default, generational garbage collection to reclaim disk
// space.
// DoltGCFull runs a full Dolt garbage collection across all storage
// generations.
// ListRemoteRefs returns the names of all cached remote-tracking refs.
// PruneRemoteRefs deletes all cached remote-tracking refs so a post-squash GC
// can reclaim the history they anchor (bd-agctw). Returns the deleted names.
// ListTags returns the names of all Dolt tags.
// ImportJSONLData atomically checks if the database is empty and, if so,
// imports parsed issues and config key/value pairs in a single transaction.
// Returns the count of issues imported, or 0 if the database was not empty.
// Does NOT issue DOLT_COMMIT — the caller is responsible for committing
// (e.g. via the PersistentPostRun auto-commit hook).
func (s *Store) ImportJSONLData(
	ctx context.Context,
	issues []*types.Issue,
	configEntries map[string]string,
	actor string,
) (int, error) {
	var imported int
	err := s.withConn(ctx, true, func(tx *sql.Tx) error {
		// Atomically check: is the database empty?
		stats := &types.Statistics{}
		if err := issueops.ScanIssueCountsInTx(ctx, tx, stats); err != nil {
			return fmt.Errorf("checking issue count: %w", err)
		}
		if stats.TotalIssues > 0 {
			return nil // database is not empty — skip import
		}

		// Import config entries (memories, etc.)
		for key, value := range configEntries {
			if err := issueops.SetConfigInTx(ctx, tx, key, value); err != nil {
				return fmt.Errorf("importing config %q: %w", key, err)
			}
		}

		if len(issues) == 0 {
			return nil
		}

		// Auto-detect prefix from first issue if not already provided
		if _, hasPrefix := configEntries["issue_prefix"]; !hasPrefix {
			firstPrefix := utils.ExtractIssuePrefix(issues[0].ID)
			if firstPrefix != "" {
				if err := issueops.SetConfigInTx(ctx, tx, "issue_prefix", firstPrefix); err != nil {
					return fmt.Errorf("setting issue_prefix: %w", err)
				}
			}
		}

		// Create all issues in the same transaction
		if err := issueops.CreateIssuesInTx(ctx, tx, issues, actor, storage.BatchCreateOptions{
			SkipPrefixValidation: true,
			// Defense-in-depth (GH#3955): the embedded fast-path is the primary
			// auto-import route for 1.0+ users and is gated by the in-transaction
			// emptiness check above. Make it insert-if-new too so a regression in
			// that check cannot clobber live rows — matching the server-mode
			// fallback's conflict-skip behavior.
			ConflictSkip: true,
		}); err != nil {
			return err
		}

		imported = len(issues)
		return nil
	})
	return imported, err
}

// Flatten squashes all Dolt commit history into a single commit.
// Pins a single *sql.Conn for session-scoped stored procedures.
// Compact squashes old Dolt commits while preserving recent ones.
// Pins a single *sql.Conn for session-scoped stored procedures.
// Path returns the embedded dolt data directory (.beads/embeddeddolt/).
// CLIDir returns the directory for dolt CLI operations (push/pull/remote).
// This is the actual database directory within the data dir.
// ActiveDatabaseSize returns the approximate size of this store's active
// database directory. Sibling databases under the embedded data root are not
// part of the result.
// CommitPending commits all working set changes and reports whether a commit
// actually landed. It gets that from commitAll's returned bool rather than
// inspecting Commit's error or reading HEAD before and after: as of GH#3886,
// Commit itself tolerates Dolt's "nothing to commit" response (matching the
// server store) and returns nil for it, so an error-based check here would
// report every clean-store call as "committed", and a HEAD-before/HEAD-after
// comparison would cost two extra engine opens on every call (this runs on
// every embedded pull/sync) and race against any concurrent HEAD movement.
// PreviousExternalRef returns the external_ref value recorded for issueID
// as of the most recent commit at or before asOf.
// Implements storage.ExternalRefHistoryQuerier.
func (s *Store) DeleteIssues(ctx context.Context, ids []string, cascade bool, force bool, dryRun bool) (*types.DeleteIssuesResult, error) {
	var result *types.DeleteIssuesResult
	err := s.withConn(ctx, !dryRun, func(tx *sql.Tx) error {
		var err error
		// storage.DeleteIssues carries no actor, so the journal rows record none.
		result, err = issueops.DeleteIssuesInTx(ctx, tx, ids, cascade, force, dryRun, "")
		return err
	})
	return result, err
}

func (s *Store) DeleteIssuesBySourceRepo(ctx context.Context, sourceRepo string) (int, error) {
	var count int
	err := s.withConn(ctx, true, func(tx *sql.Tx) error {
		var err error
		count, err = issueops.DeleteIssuesBySourceRepoInTx(ctx, tx, sourceRepo)
		return err
	})
	return count, err
}

func (s *Store) UpdateIssueID(ctx context.Context, oldID, newID string, issue *types.Issue, actor string) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.UpdateIssueIDInTx(ctx, tx, oldID, newID, issue, actor)
	})
}

// ClaimIssue is implemented in issues.go.

func (s *Store) PromoteFromEphemeral(ctx context.Context, id string, actor string) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.PromoteFromEphemeralInTx(ctx, tx, id, actor)
	})
}

// PartitionWispIDs reports which of ids currently live in the wisps table
// (batched membership query; IDs absent from the wisps table are returned as
// permanent). Export's plane-marker stamping uses this to tell an unpromoted
// no-history wisp apart from a promoted one, which row flags cannot do
// (bd-r9uce).
func (s *Store) PartitionWispIDs(ctx context.Context, ids []string) (wispIDs, permIDs []string, err error) {
	err = s.withConn(ctx, false, func(tx *sql.Tx) error {
		var inErr error
		wispIDs, permIDs, inErr = issueops.PartitionWispIDsInTx(ctx, tx, ids)
		return inErr
	})
	return wispIDs, permIDs, err
}

// GetNextChildID is implemented in child_id.go.

// ---------------------------------------------------------------------------
// storage.DependencyQueryStore
// ---------------------------------------------------------------------------

func (s *Store) GetDependencyRecords(ctx context.Context, issueID string) ([]*types.Dependency, error) {
	var result []*types.Dependency
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		m, err := issueops.GetDependencyRecordsForIssuesInTx(ctx, tx, []string{issueID})
		if err != nil {
			return err
		}
		result = m[issueID]
		return nil
	})
	return result, err
}

// GetDependentRecords returns raw dependency rows whose target is targetID,
// without hydrating the source issues. Delegates to shared query logic.
func (s *Store) GetDependentRecords(ctx context.Context, targetID string, depType string, limit int, afterID string) ([]*types.Dependency, error) {
	var result []*types.Dependency
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetDependentRecordsInTx(ctx, tx, targetID, depType, limit, afterID)
		return err
	})
	return result, err
}

// CountDependentRecords returns the total inbound-edge count of targetID across
// both dependency tables. Delegates to issueops.CountDependentRecordsInTx.
func (s *Store) CountDependentRecords(ctx context.Context, targetID string, depType string) (int, error) {
	var n int
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		n, err = issueops.CountDependentRecordsInTx(ctx, tx, targetID, depType)
		return err
	})
	return n, err
}

// IsBlocked is implemented in issues.go.

// GetNewlyUnblockedByClose is implemented in issues.go.

// DetectCycles is implemented in dependencies.go.

func (s *Store) FindWispDependentsRecursive(ctx context.Context, ids []string) (map[string]bool, error) {
	var result map[string]bool
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.FindWispDependentsRecursiveInTx(ctx, tx, ids)
		return err
	})
	return result, err
}

// ---------------------------------------------------------------------------
// storage.AnnotationQueryStore
// ---------------------------------------------------------------------------

func (s *Store) AddComment(ctx context.Context, issueID, actor, comment string) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.AddCommentEventInTx(ctx, tx, issueID, actor, comment)
	})
}

func (s *Store) ImportIssueComment(ctx context.Context, issueID, author, text string, createdAt time.Time) (*types.Comment, error) {
	var result *types.Comment
	err := s.withConn(ctx, true, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.ImportIssueCommentInTx(ctx, tx, issueID, author, text, createdAt)
		return err
	})
	return result, err
}

func (s *Store) GetCommentsForIssues(ctx context.Context, issueIDs []string) (map[string][]*types.Comment, error) {
	var result map[string][]*types.Comment
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetCommentsForIssuesInTx(ctx, tx, issueIDs)
		return err
	})
	return result, err
}

// ---------------------------------------------------------------------------
// storage.ConfigMetadataStore
// ---------------------------------------------------------------------------

func (s *Store) DeleteConfig(ctx context.Context, key string) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.DeleteConfigInTx(ctx, tx, key)
	})
}

func (s *Store) GetCustomStatuses(ctx context.Context) ([]string, error) {
	detailed, err := s.GetCustomStatusesDetailed(ctx)
	if err != nil {
		return nil, err
	}
	return types.CustomStatusNames(detailed), nil
}

func (s *Store) GetCustomStatusesDetailed(ctx context.Context) ([]types.CustomStatus, error) {
	var result []types.CustomStatus
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var txErr error
		result, txErr = issueops.ResolveCustomStatusesDetailedInTx(ctx, tx)
		return txErr
	})
	if err != nil {
		// DB unavailable — fall back to config.yaml.
		if yamlStatuses := config.GetCustomStatusesFromYAML(); len(yamlStatuses) > 0 {
			return issueops.ParseStatusFallback(yamlStatuses), nil
		}
		return nil, nil
	}
	return result, nil
}

func (s *Store) GetCustomTypes(ctx context.Context) ([]string, error) {
	var result []string
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var txErr error
		result, txErr = issueops.ResolveCustomTypesInTx(ctx, tx)
		return txErr
	})
	if err != nil {
		// DB unavailable — fall back to config.yaml.
		if yamlTypes := config.GetCustomTypesFromYAML(); len(yamlTypes) > 0 {
			return yamlTypes, nil
		}
		return nil, err
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// storage.CompactionStore
// ---------------------------------------------------------------------------

func (s *Store) CheckEligibility(ctx context.Context, issueID string, tier int) (bool, string, error) {
	var eligible bool
	var reason string
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		eligible, reason, err = issueops.CheckEligibilityInTx(ctx, tx, issueID, tier)
		return err
	})
	return eligible, reason, err
}

func (s *Store) ApplyCompaction(ctx context.Context, issueID string, tier int, originalSize int, _ int, commitHash string) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.ApplyCompactionInTx(ctx, tx, issueID, tier, originalSize, commitHash)
	})
}

func (s *Store) SnapshotIssue(ctx context.Context, issueID string, tier int) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.SnapshotIssueInTx(ctx, tx, issueID, tier)
	})
}

func (s *Store) GetCompactionSnapshot(ctx context.Context, issueID string) (*types.IssueSnapshot, error) {
	var snap *types.IssueSnapshot
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		snap, err = issueops.GetLatestSnapshotInTx(ctx, tx, issueID)
		return err
	})
	return snap, err
}

func (s *Store) RestoreFromSnapshot(ctx context.Context, issueID string) (*types.IssueSnapshot, error) {
	var snap *types.IssueSnapshot
	err := s.withConn(ctx, true, func(tx *sql.Tx) error {
		var err error
		snap, err = issueops.RestoreFromSnapshotInTx(ctx, tx, issueID)
		return err
	})
	return snap, err
}

func (s *Store) GetTier1Candidates(ctx context.Context) ([]*types.CompactionCandidate, error) {
	var result []*types.CompactionCandidate
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetTier1CandidatesInTx(ctx, tx)
		return err
	})
	return result, err
}

func (s *Store) GetTier2Candidates(ctx context.Context) ([]*types.CompactionCandidate, error) {
	var result []*types.CompactionCandidate
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetTier2CandidatesInTx(ctx, tx)
		return err
	})
	return result, err
}

// ---------------------------------------------------------------------------
// storage.AdvancedQueryStore
// ---------------------------------------------------------------------------

func (s *Store) GetRepoMtime(ctx context.Context, repoPath string) (int64, error) {
	var result int64
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetRepoMtimeInTx(ctx, tx, repoPath)
		return err
	})
	return result, err
}

func (s *Store) SetRepoMtime(ctx context.Context, repoPath, jsonlPath string, mtimeNs int64) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.SetRepoMtimeInTx(ctx, tx, repoPath, jsonlPath, mtimeNs)
	})
}

func (s *Store) ClearRepoMtime(ctx context.Context, repoPath string) error {
	return s.withConn(ctx, true, func(tx *sql.Tx) error {
		return issueops.ClearRepoMtimeInTx(ctx, tx, repoPath)
	})
}

// GetMoleculeProgress is implemented in queries.go.

func (s *Store) GetMoleculeLastActivity(ctx context.Context, moleculeID string) (*types.MoleculeLastActivity, error) {
	var result *types.MoleculeLastActivity
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetMoleculeLastActivityInTx(ctx, tx, moleculeID)
		return err
	})
	return result, err
}

func (s *Store) GetStaleIssues(ctx context.Context, filter types.StaleFilter) ([]*types.Issue, error) {
	var result []*types.Issue
	err := s.withConn(ctx, false, func(tx *sql.Tx) error {
		var err error
		result, err = issueops.GetStaleIssuesInTx(ctx, tx, filter)
		return err
	})
	return result, err
}
