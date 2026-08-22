//go:build !windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// replaceDurableRegistryFile durably replaces path with the contents of the
// already-synced temporary file at tempPath. On POSIX systems renaming is
// atomic, but the rename itself must additionally be fsync'd via the
// containing directory to survive a crash.
func replaceDurableRegistryFile(tempPath, path string) error {
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncRegistryDirectory(filepath.Dir(path))
}

func syncRegistryDirectory(path string) (err error) {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open workspace registry directory: %w", err)
	}
	defer func() {
		if closeErr := directory.Close(); closeErr != nil {
			closeErr = fmt.Errorf("close workspace registry directory: %w", closeErr)
			if err == nil {
				err = closeErr
			} else {
				err = errors.Join(err, closeErr)
			}
		}
	}()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync workspace registry directory: %w", err)
	}
	return nil
}
