//go:build !windows

package repository

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func replaceDurableStateFile(tempPath, path string) error {
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return durableWriteCommittedError{err: err}
	}
	return nil
}

func syncDirectory(path string) (err error) {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer func() {
		if closeErr := directory.Close(); closeErr != nil {
			closeErr = fmt.Errorf("close directory: %w", closeErr)
			if err == nil {
				err = closeErr
			} else {
				err = errors.Join(err, closeErr)
			}
		}
	}()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}
