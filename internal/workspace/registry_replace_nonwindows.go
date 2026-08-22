//go:build !windows

package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// replaceDurableFile durably replaces path with the contents of the already-
// synced temporary file at tempPath. On POSIX systems renaming is atomic, but
// the rename itself must additionally be fsync'd via the containing directory
// to survive a crash.
func replaceDurableFile(tempPath, path, label string) error {
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path), label)
}

func syncDirectory(path, label string) (err error) {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s directory: %w", label, err)
	}
	defer func() {
		if closeErr := directory.Close(); closeErr != nil {
			closeErr = fmt.Errorf("close %s directory: %w", label, closeErr)
			if err == nil {
				err = closeErr
			} else {
				err = errors.Join(err, closeErr)
			}
		}
	}()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync %s directory: %w", label, err)
	}
	return nil
}
