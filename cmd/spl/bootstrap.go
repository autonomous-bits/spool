package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/repository/initialization"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
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
	return repositoryStateDirFrom(workingDirectory)
}

func repositoryStateDirFrom(workingDirectory string) (string, error) {
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
