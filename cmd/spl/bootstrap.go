package main

import (
	"context"
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
// --state-dir flag or SPOOL_DIR. It reports ok=false when no override applies,
// so callers fall back to manifest and local-repository discovery.
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

func repositoryStateDirFrom(workingDirectory string) (string, error) {
	directory, err := filepath.Abs(workingDirectory)
	if err != nil {
		return "", err
	}
	_, manifest, found, err := repository.DiscoverWorkspaceManifest(directory)
	if err != nil {
		return "", err
	}
	if found {
		root, err := repository.WorkspaceStorageRoot()
		if err != nil {
			return "", fmt.Errorf("resolve workspace manifest storage: %w", err)
		}
		stateDir, err := repository.FindWorkspaceByID(root, manifest.WorkspaceID)
		if err != nil {
			return "", fmt.Errorf("resolve workspace manifest: %w", err)
		}
		return stateDir, nil
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
