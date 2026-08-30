package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
)

const (
	CurrentManifestVersion = 1
	manifestDirectory      = ".spl"
	manifestFilename       = "config.toml"
	manifestLockFilename   = "manifest.lock"
)

var (
	ErrInvalidManifest     = errors.New("workspace manifest is invalid")
	ErrManifestNotFound    = errors.New("workspace manifest was not found")
	ErrManifestConflict    = errors.New("workspace manifest binds a different workspace")
	ErrInvalidRepositoryID = errors.New("repository identity is invalid")
)

// Manifest declares the portable workspace binding for a checkout.
type Manifest struct {
	FormatVersion int    `toml:"format_version"`
	RepositoryID  string `toml:"repository_id"`
	WorkspaceID   ID     `toml:"workspace_id"`
}

// Validate rejects host paths and incomplete or unsupported manifest data.
func (manifest Manifest) Validate() error {
	if manifest.FormatVersion != CurrentManifestVersion {
		return fmt.Errorf("%w: format version %d is unsupported", ErrInvalidManifest, manifest.FormatVersion)
	}
	if err := validateRepositoryID(manifest.RepositoryID); err != nil {
		return err
	}
	if err := manifest.WorkspaceID.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidManifest, err)
	}
	return nil
}

// ManifestPath returns the checkout-owned manifest path.
func ManifestPath(repositoryRoot string) (string, error) {
	if !filepath.IsAbs(repositoryRoot) {
		return "", fmt.Errorf("%w: repository root must be absolute", ErrInvalidManifest)
	}
	return filepath.Join(filepath.Clean(repositoryRoot), manifestDirectory, manifestFilename), nil
}

// WriteManifest atomically writes a validated checkout manifest. Existing local
// repository control state is never overwritten.
func WriteManifest(repositoryRoot string, manifest Manifest) (err error) {
	if err := manifest.Validate(); err != nil {
		return err
	}
	path, err := ManifestPath(repositoryRoot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create workspace manifest directory: %w", err)
	}
	lock := flock.New(filepath.Join(filepath.Dir(path), manifestLockFilename))
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("lock workspace manifest: %w", err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			unlockErr = fmt.Errorf("unlock workspace manifest: %w", unlockErr)
			if err == nil {
				err = unlockErr
			} else {
				err = errors.Join(err, unlockErr)
			}
		}
	}()
	if existing, readErr := os.ReadFile(path); readErr == nil {
		var decoded Manifest
		if err := toml.Unmarshal(existing, &decoded); err != nil {
			return fmt.Errorf("%w: decode existing %q: %w", ErrInvalidManifest, path, err)
		}
		if decoded.WorkspaceID == "" {
			return fmt.Errorf("%w: %q contains local repository control state", ErrManifestConflict, path)
		}
		if err := decoded.Validate(); err != nil {
			return fmt.Errorf("%w: existing %q: %w", ErrInvalidManifest, path, err)
		}
		if decoded != manifest {
			return fmt.Errorf("%w: %q already binds workspace %q", ErrManifestConflict, path, decoded.WorkspaceID)
		}
		return nil
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("read existing workspace manifest %q: %w", path, readErr)
	}
	data, err := toml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode workspace manifest: %w", err)
	}
	return writeDurableFile(path, data, ".manifest-*", "workspace manifest")
}

// DiscoverManifest searches workingDirectory and its ancestors for a workspace
// manifest. A local Spool repository config without workspace_id is ignored.
func DiscoverManifest(workingDirectory string) (repositoryRoot string, manifest Manifest, found bool, err error) {
	directory, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", Manifest{}, false, fmt.Errorf("make working directory absolute: %w", err)
	}
	for {
		path := filepath.Join(directory, manifestDirectory, manifestFilename)
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			var decoded Manifest
			if err := toml.Unmarshal(data, &decoded); err != nil {
				return "", Manifest{}, false, fmt.Errorf("%w: decode %q: %w", ErrInvalidManifest, path, err)
			}
			if decoded.WorkspaceID == "" {
				// This is the existing local repository control-state config.
				return "", Manifest{}, false, nil
			}
			if err := decoded.Validate(); err != nil {
				return "", Manifest{}, false, fmt.Errorf("%w: %q: %w", ErrInvalidManifest, path, err)
			}
			return directory, decoded, true, nil
		}
		if !os.IsNotExist(readErr) {
			return "", Manifest{}, false, fmt.Errorf("read workspace manifest %q: %w", path, readErr)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", Manifest{}, false, nil
		}
		directory = parent
	}
}

func validateRepositoryID(value string) error {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) {
		return fmt.Errorf("%w: must be a non-empty portable identifier", ErrInvalidRepositoryID)
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("%w: %q contains an invalid path component", ErrInvalidRepositoryID, value)
		}
	}
	return nil
}

func writeDurableFile(path string, data []byte, tempPattern, label string) (err error) {
	temp, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return fmt.Errorf("create %s temporary file: %w", label, err)
	}
	tempPath := temp.Name()
	defer func() {
		if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
			removeErr = fmt.Errorf("remove %s temporary file: %w", label, removeErr)
			if err == nil {
				err = removeErr
			} else {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if _, err := temp.Write(data); err != nil {
		return closeTempFileAfterFailure(temp, fmt.Errorf("write %s: %w", label, err), label)
	}
	if err := temp.Sync(); err != nil {
		return closeTempFileAfterFailure(temp, fmt.Errorf("sync %s: %w", label, err), label)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s temporary file: %w", label, err)
	}
	if err := replaceDurableFile(tempPath, path, label); err != nil {
		return fmt.Errorf("replace %s: %w", label, err)
	}
	return nil
}

func closeTempFileAfterFailure(temp *os.File, operationErr error, label string) error {
	if err := temp.Close(); err != nil {
		return errors.Join(operationErr, fmt.Errorf("close %s temporary file: %w", label, err))
	}
	return operationErr
}
