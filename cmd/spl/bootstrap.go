package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/initialization"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/autonomous-bits/spool/internal/workspace"
	"github.com/spf13/cobra"
)

const (
	stateDirFlagName = "--state-dir"
	envStateDir      = "SPOOL_DIR"
	envWorkspaceName = "SPOOL_WORKSPACE"
)

func bootstrapRootCommand(stdout io.Writer, stateDir string) (*cobra.Command, func() error) {
	var closeRepository func() error
	toolProvider := func() (*resolve.ResolveTool, error) {
		tool, close, err := openPersistentTool(stateDir)
		if err != nil {
			return nil, err
		}
		closeRepository = close
		return tool, nil
	}
	initialize := func() (*repository.Repository, error) {
		repo, err := initialization.Initialize(stateDir)
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
	return newRootCommandWithLifecycle(stdout, toolProvider, initialize, func() (*resolve.FsckTool, error) {
		return resolve.NewPersistentFsckTool(stateDir), nil
	}), close
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
// --state-dir flag or the SPOOL_DIR/SPOOL_WORKSPACE environment variables, in
// that priority order. It reports ok=false when no override applies, so
// callers fall back to registry-based path-prefix discovery.
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
	return "", false, nil
}

// parseFlagValue scans args for a "--name value" or "--name=value" occurrence
// of the named flag and reports its value.
func parseFlagValue(args []string, name string) (string, bool) {
	prefix := name + "="
	for index, arg := range args {
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
	root, err := workspace.StorageRoot()
	if err != nil {
		return "", err
	}
	name, err := workspace.ParseName(rawName)
	if err != nil {
		return "", fmt.Errorf("%s: %w", envWorkspaceName, err)
	}
	registry, err := workspace.LoadRegistry(root)
	if err != nil {
		return "", err
	}
	entry, exists := registry.Workspaces[name]
	if !exists {
		return "", fmt.Errorf("%s: workspace %q is not registered", envWorkspaceName, name)
	}
	return entry.StateDir, nil
}

func repositoryStateDirFrom(workingDirectory string) (string, error) {
	directory, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", err
	}
	root, err := workspace.StorageRoot()
	if err == nil {
		match, err := workspace.FindWorkspace(root, directory)
		if err == nil {
			return match.Workspace.StateDir, nil
		}
		if !errors.Is(err, workspace.ErrWorkspaceNotFound) {
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
