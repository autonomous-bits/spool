//go:build windows

package workspace

import "golang.org/x/sys/windows"

// replaceDurableRegistryFile durably replaces path with the contents of the
// already-synced temporary file at tempPath. Windows cannot fsync a
// directory handle, and a plain rename is not guaranteed durable, so this
// uses MoveFileEx with MOVEFILE_WRITE_THROUGH to flush the rename to disk
// before returning, mirroring internal/repository's Windows durable replace.
func replaceDurableRegistryFile(tempPath, path string) error {
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
