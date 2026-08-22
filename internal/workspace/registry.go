package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
)

const (
	// CurrentRegistryVersion is the only registry format this package reads and writes.
	CurrentRegistryVersion = 1
	registryFilename       = "registry.toml"
	registryLockFilename   = "registry.lock"
)

var (
	// ErrInvalidRegistry reports invalid registry structure or an unsupported version.
	ErrInvalidRegistry = errors.New("workspace registry is invalid")
	// ErrPathAlreadyAttached reports an attempt to attach a repository to a second workspace.
	ErrPathAlreadyAttached = errors.New("repository path is already attached to another workspace")
	// ErrWorkspaceNotFound reports that no registered workspace owns a working directory.
	ErrWorkspaceNotFound = errors.New("no workspace matches working directory")
)

// Registry is the versioned central workspace registry stored at registry.toml.
// Workspace map keys are stable workspace slugs; Workspace.Name is the display name.
type Registry struct {
	Version    int                `toml:"version"`
	Workspaces map[Name]Workspace `toml:"workspaces"`
}

// NewRegistry returns an empty registry using the current format version.
func NewRegistry() Registry {
	return Registry{
		Version:    CurrentRegistryVersion,
		Workspaces: make(map[Name]Workspace),
	}
}

// RegistryPath returns the registry file below root.
func RegistryPath(root string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: registry root must be absolute", ErrInvalidStorageRoot)
	}
	return filepath.Join(filepath.Clean(root), registryFilename), nil
}

// RegistryLockPath returns the registry lock file below root.
func RegistryLockPath(root string) (string, error) {
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: registry root must be absolute", ErrInvalidStorageRoot)
	}
	return filepath.Join(filepath.Clean(root), registryLockFilename), nil
}

// LoadRegistry reads the registry rooted at root while holding registry.lock.
// A missing registry is represented by an empty current-version registry.
func LoadRegistry(root string) (registry Registry, err error) {
	err = withRegistryLock(root, func(path string) error {
		registry, err = loadRegistry(path)
		return err
	})
	return registry, err
}

// UpdateRegistry serializes mutations through registry.lock and durably persists
// the complete replacement registry. The mutation must not retain registry.
func UpdateRegistry(root string, mutate func(*Registry) error) error {
	if mutate == nil {
		return errors.New("workspace registry mutation is nil")
	}
	return withRegistryLock(root, func(path string) error {
		registry, err := loadRegistry(path)
		if err != nil {
			return err
		}
		if err := mutate(&registry); err != nil {
			return err
		}
		if err := normalizeAndValidateRegistry(&registry); err != nil {
			return err
		}
		data, err := toml.Marshal(registry)
		if err != nil {
			return fmt.Errorf("encode workspace registry: %w", err)
		}
		if err := writeDurableRegistry(path, data); err != nil {
			return err
		}
		return nil
	})
}

// AttachPath canonicalizes path and attaches it to workspace. The workspace must
// already be present in the registry.
func AttachPath(root string, workspaceName Name, path string) error {
	return UpdateRegistry(root, func(registry *Registry) error {
		if err := workspaceName.Validate(); err != nil {
			return err
		}
		workspace, ok := registry.Workspaces[workspaceName]
		if !ok {
			return fmt.Errorf("%w: workspace %q does not exist", ErrInvalidWorkspace, workspaceName)
		}
		canonicalPath, err := CanonicalPath(path)
		if err != nil {
			return err
		}
		for otherName, other := range registry.Workspaces {
			if otherName == workspaceName {
				continue
			}
			for _, attachedPath := range other.Paths {
				if attachedPath == canonicalPath {
					return fmt.Errorf("%w: %q belongs to workspace %q", ErrPathAlreadyAttached, canonicalPath, otherName)
				}
			}
		}
		for _, attachedPath := range workspace.Paths {
			if attachedPath == canonicalPath {
				return nil
			}
		}
		workspace.Paths = append(workspace.Paths, canonicalPath)
		registry.Workspaces[workspaceName] = workspace
		return nil
	})
}

// CanonicalPath returns path as an absolute, symlink-evaluated, clean path.
func CanonicalPath(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make repository path absolute: %w", err)
	}
	evaluatedPath, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", fmt.Errorf("evaluate repository path symlinks %q: %w", absolutePath, err)
	}
	return filepath.Clean(evaluatedPath), nil
}

func withRegistryLock(root string, operation func(registryPath string) error) (err error) {
	registryPath, err := RegistryPath(root)
	if err != nil {
		return err
	}
	lockPath, err := RegistryLockPath(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(registryPath), 0o700); err != nil {
		return fmt.Errorf("create workspace registry directory: %w", err)
	}

	lock := flock.New(lockPath)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("lock workspace registry: %w", err)
	}
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			unlockErr = fmt.Errorf("unlock workspace registry: %w", unlockErr)
			if err == nil {
				err = unlockErr
			} else {
				err = errors.Join(err, unlockErr)
			}
		}
	}()
	return operation(registryPath)
}

func loadRegistry(path string) (Registry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return NewRegistry(), nil
	}
	if err != nil {
		return Registry{}, fmt.Errorf("read workspace registry: %w", err)
	}

	var registry Registry
	if err := toml.Unmarshal(data, &registry); err != nil {
		return Registry{}, fmt.Errorf("decode workspace registry: %w", err)
	}
	if err := validateRegistry(registry); err != nil {
		return Registry{}, err
	}
	if registry.Workspaces == nil {
		registry.Workspaces = make(map[Name]Workspace)
	}
	return registry, nil
}

func normalizeAndValidateRegistry(registry *Registry) error {
	if registry.Version == 0 {
		registry.Version = CurrentRegistryVersion
	}
	if registry.Workspaces == nil {
		registry.Workspaces = make(map[Name]Workspace)
	}
	for name, workspace := range registry.Workspaces {
		paths := make([]string, 0, len(workspace.Paths))
		seen := make(map[string]struct{}, len(workspace.Paths))
		for _, path := range workspace.Paths {
			canonicalPath, err := CanonicalPath(path)
			if err != nil {
				return err
			}
			if _, duplicate := seen[canonicalPath]; duplicate {
				continue
			}
			seen[canonicalPath] = struct{}{}
			paths = append(paths, canonicalPath)
		}
		workspace.Paths = paths
		registry.Workspaces[name] = workspace
	}
	return validateRegistry(*registry)
}

func validateRegistry(registry Registry) error {
	if registry.Version != CurrentRegistryVersion {
		return fmt.Errorf("%w: version %d is unsupported", ErrInvalidRegistry, registry.Version)
	}
	paths := make(map[string]Name)
	for name, workspace := range registry.Workspaces {
		if err := name.Validate(); err != nil {
			return fmt.Errorf("%w: workspace key %q: %w", ErrInvalidRegistry, name, err)
		}
		if err := workspace.Validate(); err != nil {
			return fmt.Errorf("%w: workspace %q: %w", ErrInvalidRegistry, name, err)
		}
		for _, path := range workspace.Paths {
			if !filepath.IsAbs(path) {
				return fmt.Errorf("%w: attached path %q for workspace %q must be absolute", ErrInvalidRegistry, path, name)
			}
			canonicalPath, err := CanonicalPath(path)
			if err != nil {
				return fmt.Errorf("canonicalize attached path for workspace %q: %w", name, err)
			}
			if path != canonicalPath {
				return fmt.Errorf("%w: attached path %q for workspace %q is not canonical", ErrInvalidRegistry, path, name)
			}
			if previousName, exists := paths[path]; exists && previousName != name {
				return fmt.Errorf("%w: %q belongs to both %q and %q", ErrPathAlreadyAttached, path, previousName, name)
			}
			paths[path] = name
		}
	}
	return nil
}

func writeDurableRegistry(path string, data []byte) (err error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".registry-*")
	if err != nil {
		return fmt.Errorf("create workspace registry temporary file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		if removeErr := os.Remove(tempPath); removeErr != nil && !os.IsNotExist(removeErr) {
			removeErr = fmt.Errorf("remove workspace registry temporary file: %w", removeErr)
			if err == nil {
				err = removeErr
			} else {
				err = errors.Join(err, removeErr)
			}
		}
	}()
	if _, err := temp.Write(data); err != nil {
		return closeRegistryTempAfterFailure(temp, fmt.Errorf("write workspace registry: %w", err))
	}
	if err := temp.Sync(); err != nil {
		return closeRegistryTempAfterFailure(temp, fmt.Errorf("sync workspace registry: %w", err))
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close workspace registry: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace workspace registry: %w", err)
	}
	if err := syncRegistryDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("sync workspace registry directory: %w", err)
	}
	return nil
}

func closeRegistryTempAfterFailure(temp *os.File, operationErr error) error {
	if err := temp.Close(); err != nil {
		return errors.Join(operationErr, fmt.Errorf("close workspace registry temporary file: %w", err))
	}
	return operationErr
}
