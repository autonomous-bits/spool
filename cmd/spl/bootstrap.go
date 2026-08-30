package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

const (
	stateDirFlagName = "--state-dir"
	envStateDir      = "SPOOL_DIR"
	envWorkspaceName = "SPOOL_WORKSPACE"
)

func bootstrapRootCommand(stdout io.Writer, stateDir string) (*cobra.Command, func() error) {
	var closeRepository func() error
	repoProvider := func() (*repository.Repository, error) {
		repo, err := repository.OpenRepository(stateDir)
		if err != nil {
			return nil, err
		}
		closeRepository = repo.Close
		return repo, nil
	}
	toolProvider := func() (*resolve.ResolveTool, error) {
		repo, err := repoProvider()
		if err != nil {
			return nil, err
		}
		return resolve.NewResolveTool(repo), nil
	}
	initialize := func() (*repository.Repository, error) {
		repo, err := repository.InitializeRepository(stateDir)
		if err != nil {
			return nil, err
		}
		closeRepository = repo.Close
		return repo, nil
	}
	close := func() error {
		if closeRepository == nil {
			return nil
		}
		return closeRepository()
	}
	return newRootCommandWithLifecycle(stdout, repoProvider, toolProvider, initialize, func(ctx context.Context) (repository.FsckResult, error) {
		if err := ctx.Err(); err != nil {
			return repository.FsckResult{}, err
		}
		return repository.FsckRepository(stateDir)
	}), close
}

func openPersistentRepository(stateDir string) (*repository.Repository, func() error, error) {
	repo, err := repository.OpenRepository(stateDir)
	if err != nil {
		return nil, nil, err
	}
	return repo, repo.Close, nil
}

func openPersistentTool(stateDir string) (*resolve.ResolveTool, func() error, error) {
	repo, err := repository.OpenRepository(stateDir)
	if err != nil {
		return nil, nil, err
	}
	return resolve.NewResolveTool(repo), repo.Close, nil
}

func repositoryStateDir() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return repositoryStateDirFromArgs(os.Args[1:], os.LookupEnv, workingDirectory)
}

func repositoryStateDirFromArgs(args []string, lookupEnv func(string) (string, bool), workingDirectory string) (string, error) {
	if override, ok, err := stateDirOverride(args, lookupEnv); err != nil {
		return "", err
	} else if ok {
		return override, nil
	}
	return repositoryStateDirFrom(workingDirectory)
}

// stateDirOverride resolves an explicit state-directory override from the
// --state-dir flag, the SPOOL_DIR/SPOOL_WORKSPACE environment variables, or
// the persisted current-workspace preference, in that priority order. It
// reports ok=false when no override applies, so callers fall back to
// registry-based path-prefix discovery.
func stateDirOverride(args []string, lookupEnv func(string) (string, bool)) (string, bool, error) {
	if value, found := parseFlagValue(args, stateDirFlagName); found {
		if value == "" {
			return "", false, fmt.Errorf("%s requires a non-empty value", stateDirFlagName)
		}
		return value, true, nil
	}
	if value, found := lookupEnv(envStateDir); found && value != "" {
		return value, true, nil
	}
	if value, found := lookupEnv(envWorkspaceName); found && value != "" {
		stateDir, err := workspaceStateDirByName(value)
		if err != nil {
			return "", false, err
		}
		return stateDir, true, nil
	}
	// A broken persisted preference (malformed current.toml, or a slug that
	// no longer exists in the registry) must not become a hard error here:
	// repositoryStateDir runs unconditionally in main() before cobra parses
	// which subcommand was requested, so an error at this point would block
	// every spl invocation -- including "spl workspace use"/"spl workspace
	// unset", the only commands that can fix a broken preference. Treat any
	// resolution failure the same as no preference being set and fall
	// through to path-prefix discovery, mirroring how workspace.StorageRoot
	// errors are already tolerated below and in repositoryStateDirFrom.
	if root, err := repository.WorkspaceStorageRoot(); err == nil {
		if name, ok, err := repository.CurrentWorkspaceName(root); err == nil && ok {
			if stateDir, err := registeredWorkspaceStateDir(root, name, "current workspace preference"); err == nil {
				return stateDir, true, nil
			}
		}
	}
	return "", false, nil
}

// parseFlagValue scans args for a "--name value" or "--name=value" occurrence
// of the named flag and reports its value. It stops scanning at a bare "--"
// argument-terminator, matching cobra's own convention that everything after
// "--" is positional and must not be reinterpreted as a flag.
func parseFlagValue(args []string, name string) (string, bool) {
	prefix := name + "="
	for index, arg := range args {
		if arg == "--" {
			break
		}
		if arg == name {
			if index+1 < len(args) {
				return args[index+1], true
			}
			return "", true
		}
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix), true
		}
	}
	return "", false
}

func workspaceStateDirByName(rawName string) (string, error) {
	root, err := repository.WorkspaceStorageRoot()
	if err != nil {
		return "", err
	}
	name, err := repository.ParseWorkspaceName(rawName)
	if err != nil {
		return "", fmt.Errorf("%s: %w", envWorkspaceName, err)
	}
	return registeredWorkspaceStateDir(root, name, envWorkspaceName)
}

func registeredWorkspaceStateDir(root string, name repository.WorkspaceName, source string) (string, error) {
	registry, err := repository.LoadWorkspaceRegistry(root)
	if err != nil {
		return "", err
	}
	entry, exists := registry.Workspaces[name]
	if !exists {
		return "", fmt.Errorf("%s: workspace %q is not registered", source, name)
	}
	return entry.StateDir, nil
}

func repositoryStateDirFrom(workingDirectory string) (string, error) {
	directory, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", err
	}
	root, err := repository.WorkspaceStorageRoot()
	if err == nil {
		match, err := repository.FindWorkspace(root, directory)
		if err == nil {
			return match.Workspace.StateDir, nil
		}
		if !errors.Is(err, repository.ErrWorkspaceNotFound) {
			return "", err
		}
	}
	return localRepositoryStateDir(directory)
}

func localRepositoryStateDir(workingDirectory string) (string, error) {
	directory, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", err
	}
	startDirectory := directory
	for {
		stateDir := filepath.Join(directory, ".spl")
		info, statErr := os.Stat(stateDir)
		if statErr == nil && info.IsDir() {
			return stateDir, nil
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		_, statErr = os.Stat(filepath.Join(directory, "go.work"))
		if statErr == nil {
			return stateDir, nil
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return filepath.Join(startDirectory, ".spl"), nil
		}
		directory = parent
	}
}

func newLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{})).With("component", "spl")
}
