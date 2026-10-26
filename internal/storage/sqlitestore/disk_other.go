//go:build !windows

package sqlitestore

// requireLocalFixedDisk is only enforced on Windows, where the company runs.
func requireLocalFixedDisk(string) error { return nil }
