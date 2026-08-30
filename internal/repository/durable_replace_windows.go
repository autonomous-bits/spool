//go:build windows

package repository

import "golang.org/x/sys/windows"

func replaceDurableStateFile(tempPath, path string) error {
	from, err := windows.UTF16PtrFromString(tempPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

// Windows does not support syncing a directory handle; directory metadata
// durability relies on atomic file replacement and write-through flags.
func syncDirectory(string) error {
	return nil
}
