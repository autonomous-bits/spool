package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
)

const registryFilename = "registry.toml"

var (
	ErrWorkspaceNotRegistered = errors.New("workspace is not registered")
	ErrWorkspaceExists        = errors.New("workspace already exists")
)

// Registry is the central detached-state catalog. Names are used solely by
// explicit provisioning commands; checkout resolution uses only workspace IDs.
type Registry struct {
	Version    int                `toml:"version"`
	Workspaces map[Name]Workspace `toml:"workspaces"`
}

type Workspace struct {
	ID       ID     `toml:"id"`
	StateDir string `toml:"state_dir"`
}

func LoadRegistry(root string) (Registry, error) {
	if !filepath.IsAbs(root) {
		return Registry{}, fmt.Errorf("%w: registry root must be absolute", ErrInvalidStorageRoot)
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Registry{}, fmt.Errorf("create workspace registry directory: %w", err)
	}
	lock := flock.New(filepath.Join(root, "registry.lock"))
	if err := lock.RLock(); err != nil {
		return Registry{}, fmt.Errorf("lock workspace registry: %w", err)
	}
	defer func() { _ = lock.Unlock() }()
	return loadRegistry(filepath.Join(root, registryFilename))
}

// UpdateRegistry serializes catalog provisioning updates.
func UpdateRegistry(root string, update func(*Registry) error) (err error) {
	if update == nil {
		return errors.New("workspace registry mutation is nil")
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("%w: registry root must be absolute", ErrInvalidStorageRoot)
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return fmt.Errorf("create workspace registry directory: %w", err)
	}
	lock := flock.New(filepath.Join(root, "registry.lock"))
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("lock workspace registry: %w", err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			if err == nil {
				err = fmt.Errorf("unlock workspace registry: %w", unlockErr)
			} else {
				err = errors.Join(err, fmt.Errorf("unlock workspace registry: %w", unlockErr))
			}
		}
	}()
	path := filepath.Join(root, registryFilename)
	registry, err := loadRegistry(path)
	if err != nil {
		return err
	}
	if err := update(&registry); err != nil {
		return err
	}
	data, err := toml.Marshal(registry)
	if err != nil {
		return fmt.Errorf("encode workspace registry: %w", err)
	}
	return writeDurableFile(path, data, ".registry-*", "workspace registry")
}

// FindWorkspaceByID resolves a manifest's immutable workspace identity to its
// detached repository state.
func FindWorkspaceByID(root string, id ID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	catalog, err := LoadRegistry(root)
	if err != nil {
		return "", err
	}
	for _, workspace := range catalog.Workspaces {
		if workspace.ID != id {
			continue
		}
		if !filepath.IsAbs(workspace.StateDir) {
			return "", fmt.Errorf("workspace identity %q has invalid state directory", id)
		}
		return filepath.Clean(workspace.StateDir), nil
	}
	return "", fmt.Errorf("%w: workspace identity %q is not registered", ErrWorkspaceNotRegistered, id)
}

func loadRegistry(path string) (Registry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Registry{Version: 1, Workspaces: make(map[Name]Workspace)}, nil
	}
	if err != nil {
		return Registry{}, fmt.Errorf("read workspace registry: %w", err)
	}
	var registry Registry
	if err := toml.Unmarshal(data, &registry); err != nil {
		return Registry{}, fmt.Errorf("decode workspace registry: %w", err)
	}
	if registry.Workspaces == nil {
		registry.Workspaces = make(map[Name]Workspace)
	}
	return registry, nil
}
