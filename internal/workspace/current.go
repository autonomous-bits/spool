package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	currentWorkspaceVersion  = 1
	currentWorkspaceFilename = "current.toml"
)

var (
	// ErrInvalidCurrentWorkspace reports malformed or unsupported persisted
	// current-workspace preference data.
	ErrInvalidCurrentWorkspace = errors.New("current workspace preference is invalid")
	// ErrWorkspaceNotRegistered reports a workspace-name lookup that has no
	// corresponding entry in the detached workspace registry.
	ErrWorkspaceNotRegistered = errors.New("workspace is not registered")
)

type currentWorkspacePreference struct {
	Version int  `toml:"version"`
	Name    Name `toml:"name"`
}

// SetCurrentWorkspace durably records name as the active detached workspace for
// future sessions. The named workspace must already exist in the registry.
func SetCurrentWorkspace(root string, name Name) error {
	if err := name.Validate(); err != nil {
		return err
	}
	return withRegistryLock(root, func(registryPath string) error {
		registry, err := loadRegistry(registryPath)
		if err != nil {
			return err
		}
		if _, exists := registry.Workspaces[name]; !exists {
			return fmt.Errorf("%w: workspace %q is not registered", ErrWorkspaceNotRegistered, name)
		}

		data, err := toml.Marshal(currentWorkspacePreference{
			Version: currentWorkspaceVersion,
			Name:    name,
		})
		if err != nil {
			return fmt.Errorf("encode current workspace preference: %w", err)
		}
		return writeDurableFile(currentWorkspacePathFromRegistryPath(registryPath), data, ".current-*", "current workspace preference")
	})
}

// CurrentWorkspaceName reads the persisted active detached workspace
// preference. It reports ok=false when no preference has been set yet.
func CurrentWorkspaceName(root string) (name Name, ok bool, err error) {
	err = withRegistryLock(root, func(registryPath string) error {
		name, ok, err = loadCurrentWorkspacePreference(currentWorkspacePathFromRegistryPath(registryPath))
		return err
	})
	return name, ok, err
}

// ClearCurrentWorkspace removes any persisted active detached workspace
// preference. Clearing an unset preference succeeds without error.
func ClearCurrentWorkspace(root string) error {
	return withRegistryLock(root, func(registryPath string) error {
		path := currentWorkspacePathFromRegistryPath(registryPath)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove current workspace preference: %w", err)
		}
		return nil
	})
}

func currentWorkspacePathFromRegistryPath(registryPath string) string {
	return filepath.Join(filepath.Dir(registryPath), currentWorkspaceFilename)
}

func loadCurrentWorkspacePreference(path string) (Name, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read current workspace preference: %w", err)
	}

	var preference currentWorkspacePreference
	if err := toml.Unmarshal(data, &preference); err != nil {
		return "", false, fmt.Errorf("decode current workspace preference: %w", err)
	}
	if err := validateCurrentWorkspacePreference(preference); err != nil {
		return "", false, err
	}
	return preference.Name, true, nil
}

func validateCurrentWorkspacePreference(preference currentWorkspacePreference) error {
	if preference.Version != currentWorkspaceVersion {
		return fmt.Errorf("%w: version %d is unsupported", ErrInvalidCurrentWorkspace, preference.Version)
	}
	if err := preference.Name.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCurrentWorkspace, err)
	}
	return nil
}
