// Package atomicfile provides atomic file writes via temp-file + rename.
//
// Writes land in a temporary file in the same directory as the target,
// are fsynced, then atomically renamed into place. Readers never see a
// partial or truncated file — only the previous complete version or the
// new complete version.
package atomicfile

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// staleTempAge is how old a leftover ".~<base>.*" temp file must be before
// Create deletes it. A process killed between CreateTemp and Close (common
// for fire-and-forget auto-export on Windows) leaves one behind forever.
const staleTempAge = time.Hour

// sweepStale removes abandoned temp files of the same target. Best effort.
func sweepStale(dir, base string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := ".~" + base + "."
	for _, e := range ents {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > staleTempAge {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

// WriteFile atomically writes data to path with the given permissions.
// It creates an atomic Writer, copies data in via io.Copy, then
// fsyncs and renames into place.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	w, err := Create(path, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
		_ = w.Abort()
		return err
	}
	return w.Close()
}

// Writer is an io.WriteCloser that writes to a temporary file and
// atomically renames it to the target path on Close. Call Abort to
// discard the temp file without touching the target.
type Writer struct {
	target string
	f      *os.File
	perm   os.FileMode
	done   bool
}

// Create returns a Writer that will atomically replace path on Close.
// The temp file is created in the same directory as path to guarantee
// same-filesystem rename semantics.
func Create(path string, perm os.FileMode) (*Writer, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	sweepStale(dir, base)
	f, err := os.CreateTemp(dir, ".~"+base+".")
	if err != nil {
		return nil, fmt.Errorf("atomicfile: create temp: %w", err)
	}

	return &Writer{
		target: path,
		f:      f,
		perm:   perm,
	}, nil
}

// Write delegates to the underlying temp file.
func (w *Writer) Write(p []byte) (int, error) {
	return w.f.Write(p)
}

// Close fsyncs the temp file and atomically renames it to the target path.
// After Close returns successfully, the target contains exactly the data
// written. On error the temp file is removed and the target is untouched.
func (w *Writer) Close() error {
	if w.done {
		return nil
	}
	w.done = true

	// Ensure permissions before rename — CreateTemp uses 0600 by default.
	if err := w.f.Chmod(w.perm); err != nil {
		_ = w.f.Close()
		_ = os.Remove(w.f.Name())
		return fmt.Errorf("atomicfile: chmod: %w", err)
	}

	if err := w.f.Sync(); err != nil {
		_ = w.f.Close()
		_ = os.Remove(w.f.Name())
		return fmt.Errorf("atomicfile: sync: %w", err)
	}

	if err := w.f.Close(); err != nil {
		_ = os.Remove(w.f.Name())
		return fmt.Errorf("atomicfile: close: %w", err)
	}

	if err := os.Rename(w.f.Name(), w.target); err != nil {
		_ = os.Remove(w.f.Name())
		return fmt.Errorf("atomicfile: rename: %w", err)
	}

	return nil
}

// Abort discards the temp file without renaming. The target is untouched.
// Safe to call multiple times or after Close.
func (w *Writer) Abort() error {
	if w.done {
		return nil
	}
	w.done = true
	_ = w.f.Close()
	return os.Remove(w.f.Name())
}
