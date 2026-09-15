package credentials

import (
	"fmt"
	"os"
	"path/filepath"
)

// lockFileName sits beside credentials.json. Locking a separate file, rather
// than the credentials file itself, means the lock survives the rename that
// atomic writes perform.
const lockFileName = "credentials.lock"

// withLock runs fn while holding an exclusive lock on the credentials
// directory's lock file. Concurrent admiral processes serialize on it, so a
// session refresh happens once even when several commands start together.
// The lock is advisory and released on process exit, so a crash cannot wedge
// later invocations.
func withLock(configDir string, fn func() error) error {
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(configDir, lockFileName), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("opening credentials lock: %w", err)
	}
	defer f.Close() //nolint:errcheck // best-effort cleanup

	if err := lockFile(f); err != nil {
		return fmt.Errorf("locking credentials: %w", err)
	}
	defer unlockFile(f) //nolint:errcheck // released on close regardless

	return fn()
}
