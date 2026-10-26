package sqlitestore

import (
	"context"

	"github.com/steveyegge/beads/internal/storage"
	"github.com/steveyegge/beads/internal/types"
)

// Dolt-only roles (version control, remotes, backup, federation, time travel).
// SQLite has no commit graph, so each answers the upstream typed
// *storage.ErrUnsupported (same error the conformance kit expects). cmd/bd
// skips these via Store.CommitGraphUnsupported. History is in history.go (T009).

const backendName = "sqlite"

func unsupported(op string) error { return &storage.ErrUnsupported{Op: op, Backend: backendName} }

// unsupportedMethods is the allowlist given to conformance.RunUnsupportedContract.
var unsupportedMethods = map[string]string{
	"CommitAll":                  "Dolt-only",
	"Commit":                     "Dolt-only",
	"CommitWithConfig":           "Dolt-only",
	"CommitMergeResolution":      "Dolt-only",
	"AddRemote":                  "Dolt-only",
	"HasRemote":                  "Dolt-only",
	"Branch":                     "Dolt-only",
	"Checkout":                   "Dolt-only",
	"CurrentBranch":              "Dolt-only",
	"DeleteBranch":               "Dolt-only",
	"ListBranches":               "Dolt-only",
	"CommitExists":               "Dolt-only",
	"HistoricalIssueIDs":         "Dolt-only",
	"Status":                     "Dolt-only",
	"Log":                        "Dolt-only",
	"Merge":                      "Dolt-only",
	"MergeWithStrategy":          "Dolt-only",
	"RecomputeBlockedAfterMerge": "Dolt-only",
	"GetConflicts":               "Dolt-only",
	"ResolveConflicts":           "Dolt-only",
	"GetConflictRows":            "Dolt-only",
	"GetMergeBlockers":           "Dolt-only",
	"ResolveConflictRows":        "Dolt-only",
	"RemoveRemote":               "Dolt-only",
	"ListRemotes":                "Dolt-only",
	"Push":                       "Dolt-only",
	"Pull":                       "Dolt-only",
	"PullWithStrategy":           "Dolt-only",
	"PullRemoteWithStrategy":     "Dolt-only",
	"ForcePush":                  "Dolt-only",
	"PushRemote":                 "Dolt-only",
	"PullRemote":                 "Dolt-only",
	"Fetch":                      "Dolt-only",
	"PushTo":                     "Dolt-only",
	"PullFrom":                   "Dolt-only",
	"BackupAdd":                  "Dolt-only",
	"BackupSync":                 "Dolt-only",
	"BackupRemove":               "Dolt-only",
	"BackupDatabase":             "Dolt-only",
	"RestoreDatabase":            "Dolt-only",
	"AddFederationPeer":          "Dolt-only",
	"GetFederationPeer":          "Dolt-only",
	"ListFederationPeers":        "Dolt-only",
	"RemoveFederationPeer":       "Dolt-only",
	"Sync":                       "Dolt-only",
	"SyncStatus":                 "Dolt-only",
	"GetCurrentCommit":           "Dolt-only",
	"AsOf":                       "Dolt-only",
	"Diff":                       "Dolt-only",
}

func (s *Store) CommitAll(ctx context.Context, message string) (bool, error) {
	return false, unsupported("CommitAll")
}

func (s *Store) Commit(ctx context.Context, message string) error {
	return unsupported("Commit")
}

func (s *Store) CommitWithConfig(ctx context.Context, message string) error {
	return unsupported("CommitWithConfig")
}

func (s *Store) CommitMergeResolution(ctx context.Context, message string) error {
	return unsupported("CommitMergeResolution")
}

func (s *Store) AddRemote(ctx context.Context, name, url string) error {
	return unsupported("AddRemote")
}

func (s *Store) HasRemote(ctx context.Context, name string) (bool, error) {
	return false, unsupported("HasRemote")
}

func (s *Store) Branch(ctx context.Context, name string) error {
	return unsupported("Branch")
}

func (s *Store) Checkout(ctx context.Context, branch string) error {
	return unsupported("Checkout")
}

func (s *Store) CurrentBranch(ctx context.Context) (string, error) {
	return "", unsupported("CurrentBranch")
}

func (s *Store) DeleteBranch(ctx context.Context, branch string) error {
	return unsupported("DeleteBranch")
}

func (s *Store) ListBranches(ctx context.Context) ([]string, error) {
	return nil, unsupported("ListBranches")
}

func (s *Store) CommitExists(ctx context.Context, commitHash string) (bool, error) {
	return false, unsupported("CommitExists")
}

func (s *Store) HistoricalIssueIDs(ctx context.Context, ids []string) (map[string]struct{}, error) {
	return nil, unsupported("HistoricalIssueIDs")
}

func (s *Store) Status(ctx context.Context) (*storage.Status, error) {
	return nil, unsupported("Status")
}

func (s *Store) Log(ctx context.Context, limit int) ([]storage.CommitInfo, error) {
	return nil, unsupported("Log")
}

func (s *Store) Merge(ctx context.Context, branch string) ([]storage.Conflict, error) {
	return nil, unsupported("Merge")
}

func (s *Store) MergeWithStrategy(ctx context.Context, branch, strategy string) ([]storage.Conflict, error) {
	return nil, unsupported("MergeWithStrategy")
}

func (s *Store) RecomputeBlockedAfterMerge(ctx context.Context, fromCommit string) error {
	return unsupported("RecomputeBlockedAfterMerge")
}

func (s *Store) GetConflicts(ctx context.Context) ([]storage.Conflict, error) {
	return nil, unsupported("GetConflicts")
}

func (s *Store) ResolveConflicts(ctx context.Context, table string, strategy string) error {
	return unsupported("ResolveConflicts")
}

func (s *Store) GetConflictRows(ctx context.Context, table string) ([]storage.ConflictRow, error) {
	return nil, unsupported("GetConflictRows")
}

func (s *Store) GetMergeBlockers(ctx context.Context) (storage.MergeBlockers, error) {
	return nil, unsupported("GetMergeBlockers")
}

func (s *Store) ResolveConflictRows(ctx context.Context, table string, keys []string, strategy string) (int, error) {
	return 0, unsupported("ResolveConflictRows")
}

func (s *Store) RemoveRemote(ctx context.Context, name string) error {
	return unsupported("RemoveRemote")
}

func (s *Store) ListRemotes(ctx context.Context) ([]storage.RemoteInfo, error) {
	return nil, unsupported("ListRemotes")
}

func (s *Store) Push(ctx context.Context) error {
	return unsupported("Push")
}

func (s *Store) Pull(ctx context.Context) error {
	return unsupported("Pull")
}

func (s *Store) PullWithStrategy(ctx context.Context, strategy string) error {
	return unsupported("PullWithStrategy")
}

func (s *Store) PullRemoteWithStrategy(ctx context.Context, remote, strategy string) error {
	return unsupported("PullRemoteWithStrategy")
}

func (s *Store) ForcePush(ctx context.Context) error {
	return unsupported("ForcePush")
}

func (s *Store) PushRemote(ctx context.Context, remote string, force bool) error {
	return unsupported("PushRemote")
}

func (s *Store) PullRemote(ctx context.Context, remote string) error {
	return unsupported("PullRemote")
}

func (s *Store) Fetch(ctx context.Context, peer string) error {
	return unsupported("Fetch")
}

func (s *Store) PushTo(ctx context.Context, peer string) error {
	return unsupported("PushTo")
}

func (s *Store) PullFrom(ctx context.Context, peer string) ([]storage.Conflict, error) {
	return nil, unsupported("PullFrom")
}

func (s *Store) BackupAdd(ctx context.Context, name, url string) error {
	return unsupported("BackupAdd")
}

func (s *Store) BackupSync(ctx context.Context, name string) error {
	return unsupported("BackupSync")
}

func (s *Store) BackupRemove(ctx context.Context, name string) error {
	return unsupported("BackupRemove")
}

func (s *Store) BackupDatabase(ctx context.Context, dir string) error {
	return unsupported("BackupDatabase")
}

func (s *Store) RestoreDatabase(ctx context.Context, dir string, force bool) error {
	return unsupported("RestoreDatabase")
}

func (s *Store) AddFederationPeer(ctx context.Context, peer *storage.FederationPeer) error {
	return unsupported("AddFederationPeer")
}

func (s *Store) GetFederationPeer(ctx context.Context, name string) (*storage.FederationPeer, error) {
	return nil, unsupported("GetFederationPeer")
}

func (s *Store) ListFederationPeers(ctx context.Context) ([]*storage.FederationPeer, error) {
	return nil, unsupported("ListFederationPeers")
}

func (s *Store) RemoveFederationPeer(ctx context.Context, name string) error {
	return unsupported("RemoveFederationPeer")
}

func (s *Store) Sync(ctx context.Context, peer string, strategy string) (*storage.SyncResult, error) {
	return nil, unsupported("Sync")
}

func (s *Store) SyncStatus(ctx context.Context, peer string) (*storage.SyncStatus, error) {
	return nil, unsupported("SyncStatus")
}

func (s *Store) GetCurrentCommit(ctx context.Context) (string, error) {
	return "", unsupported("GetCurrentCommit")
}

func (s *Store) AsOf(ctx context.Context, issueID string, ref string) (*types.Issue, error) {
	return nil, unsupported("AsOf")
}

func (s *Store) Diff(ctx context.Context, fromRef, toRef string) ([]*storage.DiffEntry, error) {
	return nil, unsupported("Diff")
}
