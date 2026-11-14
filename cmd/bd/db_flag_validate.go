package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	dbidentifier "github.com/steveyegge/beads/internal/storage/domain/db"
)

// validateExplicitDBFlag (beads-lite-1bo5.12): an explicit --db must name an
// existing beads store. A typo'd path used to fall through to the nearest
// ancestor .beads and write there; it now fails before anything is opened.
// Accepted: a SQLite file, or a directory that is a beads dir, holds
// metadata.json, or holds Dolt data.
func validateExplicitDBFlag(p string) error {
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("--db %q: database not found", p)
		}
		return fmt.Errorf("--db %q: %w", p, err)
	}
	if info.IsDir() {
		for _, marker := range []string{"metadata.json", ".dolt", filepath.Join("dolt", ".dolt")} {
			if _, e := os.Stat(filepath.Join(p, marker)); e == nil {
				return nil
			}
		}
		if filepath.Base(p) == ".beads" {
			return nil
		}
		if beadsDir := resolveBeadsDirForDBPath(p); beadsDir != "" {
			if _, e := os.Stat(filepath.Join(beadsDir, "metadata.json")); e == nil {
				return nil
			}
		}
		return fmt.Errorf("--db %q: not a beads database (directory has no metadata.json or Dolt data)", p)
	}
	f, err := os.Open(p)
	if err != nil {
		return fmt.Errorf("--db %q: %w", p, err)
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 16)
	if _, err := io.ReadFull(f, head); err != nil || !bytes.Equal(head, []byte("SQLite format 3\x00")) {
		return fmt.Errorf("--db %q: not a beads database (not a SQLite file)", p)
	}
	return nil
}

// validateEnvDB applies validateExplicitDBFlag to BEADS_DB / BD_DB when set.
// A bare database name (proxied-server override) that is not a path is left alone.
func validateEnvDB() error {
	for _, key := range []string{"BEADS_DB", "BD_DB"} {
		v := os.Getenv(key)
		if v == "" {
			continue
		}
		if _, err := os.Stat(v); err != nil && os.IsNotExist(err) && dbidentifier.ValidateIdentifier(v) == nil {
			continue
		}
		if err := validateExplicitDBFlag(v); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	return nil
}
