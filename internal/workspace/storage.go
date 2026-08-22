package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var (
	// ErrInvalidStorageRoot reports a configured XDG data root that is not an
	// absolute filesystem path.
	ErrInvalidStorageRoot = errors.New("workspace storage root is invalid")
	// ErrStorageRootUnavailable reports that the operating system did not
	// provide the home directory needed for its default storage location.
	ErrStorageRootUnavailable = errors.New("workspace storage root is unavailable")
)

// StorageRoot returns Spool's platform-appropriate detached storage root.
//
// XDG_DATA_HOME, when configured, takes precedence on every platform. Unix
// defaults to the XDG data location; Windows uses its conventional per-user
// application-data location when no XDG override is present.
func StorageRoot() (string, error) {
	if os.Getenv("XDG_DATA_HOME") != "" || runtime.GOOS == "windows" && os.Getenv("LOCALAPPDATA") != "" {
		return storageRoot(os.Getenv, "", runtime.GOOS)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrStorageRootUnavailable, err)
	}
	return storageRoot(os.Getenv, home, runtime.GOOS)
}

func storageRoot(getenv func(string) string, home, goos string) (string, error) {
	if configured := getenv("XDG_DATA_HOME"); configured != "" {
		if !filepath.IsAbs(configured) {
			return "", fmt.Errorf("%w: XDG_DATA_HOME must be absolute", ErrInvalidStorageRoot)
		}
		return filepath.Join(configured, "spool"), nil
	}

	switch goos {
	case "windows":
		if localAppData := getenv("LOCALAPPDATA"); localAppData != "" {
			if !filepath.IsAbs(localAppData) {
				return "", fmt.Errorf("%w: LOCALAPPDATA must be absolute", ErrInvalidStorageRoot)
			}
			return filepath.Join(localAppData, "spool"), nil
		}
		if home == "" {
			return "", ErrStorageRootUnavailable
		}
		return filepath.Join(home, "AppData", "Local", "spool"), nil
	default:
		if home == "" {
			return "", ErrStorageRootUnavailable
		}
		return filepath.Join(home, ".local", "share", "spool"), nil
	}
}

// RepositoryPath returns the deterministic detached repository directory for id
// below storageRoot. It performs no filesystem access or symlink resolution.
func RepositoryPath(storageRoot string, id ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	if !filepath.IsAbs(storageRoot) {
		return "", fmt.Errorf("%w: must be absolute", ErrInvalidStorageRoot)
	}
	root := filepath.Clean(storageRoot)
	stateDir := filepath.Join(root, "repos", string(id))
	relative, err := filepath.Rel(root, stateDir)
	if err != nil || relative == ".." || filepath.IsAbs(relative) {
		return "", fmt.Errorf("%w: repository path escapes storage root", ErrInvalidStorageRoot)
	}
	return stateDir, nil
}

// CreateLocation is the resolved detached location for a workspace creation
// request.
type CreateLocation struct {
	StorageRoot string `json:"storageRoot"`
	StateDir    string `json:"stateDir"`
	ID          ID     `json:"id"`
}

// ResolveCreateLocation validates request and resolves its detached state
// directory from the current process environment.
func ResolveCreateLocation(request CreateRequest) (CreateLocation, error) {
	if err := request.Validate(); err != nil {
		return CreateLocation{}, err
	}
	root, err := StorageRoot()
	if err != nil {
		return CreateLocation{}, err
	}
	stateDir, err := RepositoryPath(root, request.ID)
	if err != nil {
		return CreateLocation{}, err
	}
	return CreateLocation{StorageRoot: root, StateDir: stateDir, ID: request.ID}, nil
}
