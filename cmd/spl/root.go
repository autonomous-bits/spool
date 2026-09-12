package main

import (
	"context"
	"errors"
	"io"

	"github.com/autonomous-bits/spool/cmd/spl/commands"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

func newRootCommand(stdout io.Writer, repo *repository.Repository) *cobra.Command {
	tool := resolve.NewResolveTool(repo)
	return newRootCommandWithLifecycle(stdout, func() (*repository.Repository, error) {
		return repo, nil
	}, func() (*resolve.ResolveTool, error) {
		return tool, nil
	}, repository.NewSeedRepository, func(ctx context.Context) (repository.FsckResult, error) {
		if err := ctx.Err(); err != nil {
			return repository.FsckResult{}, err
		}
		return repo.Fsck()
	}, func(from, to int) (*repository.MigrationResult, error) {
		if repo.StateDir() == "" {
			return nil, errors.New("repository has no durable state directory")
		}
		return repository.MigrateRepositoryFormat(repo.StateDir(), from, to)
	})
}

func newRootCommandWithLifecycle(
	stdout io.Writer,
	repoProvider func() (*repository.Repository, error),
	toolProvider func() (*resolve.ResolveTool, error),
	initialize func() (*repository.Repository, error),
	providers ...any,
) *cobra.Command {
	var fsckProvider func(context.Context) (repository.FsckResult, error)
	var migrateProvider func(from, to int) (*repository.MigrationResult, error)
	var stateDirProvider func() (string, error)
	for _, p := range providers {
		switch fn := p.(type) {
		case func(context.Context) (repository.FsckResult, error):
			fsckProvider = fn
		case func(from, to int) (*repository.MigrationResult, error):
			migrateProvider = fn
		case func() (string, error):
			stateDirProvider = fn
		}
	}
	if fsckProvider == nil {
		fsckProvider = func(ctx context.Context) (repository.FsckResult, error) {
			if err := ctx.Err(); err != nil {
				return repository.FsckResult{}, err
			}
			repo, err := repoProvider()
			if err != nil {
				return repository.FsckResult{}, err
			}
			if err := ctx.Err(); err != nil {
				return repository.FsckResult{}, err
			}
			return repo.Fsck()
		}
	}
	if migrateProvider == nil {
		migrateProvider = func(from, to int) (*repository.MigrationResult, error) {
			repo, err := repoProvider()
			if err != nil {
				return nil, err
			}
			if repo.StateDir() == "" {
				return nil, errors.New("repository has no durable state directory")
			}
			return repository.MigrateRepositoryFormat(repo.StateDir(), from, to)
		}
	}
	if stateDirProvider == nil {
		stateDirProvider = func() (string, error) {
			repo, err := repoProvider()
			if err != nil {
				return "", err
			}
			if repo.StateDir() == "" {
				return "", errors.New("repository has no durable state directory")
			}
			return repo.StateDir(), nil
		}
	}
	root := &cobra.Command{
		Use:          "spl",
		SilenceUsage: true,
	}
	root.SetOut(stdout)
	// state-dir is consumed manually in main() before the root command is
	// constructed (it determines which repository to open); it is declared
	// here only so cobra recognizes the flag instead of rejecting it.
	root.PersistentFlags().String("state-dir", "", "override the resolved Spool repository state directory")
	root.AddCommand(commands.NewInitCommand(initialize))
	root.AddCommand(commands.NewAddCommand(repoProvider))
	root.AddCommand(commands.NewStatusCommand(repoProvider))
	root.AddCommand(commands.NewCommitCommand(repoProvider))
	root.AddCommand(commands.NewBranchCommand(repoProvider))
	root.AddCommand(commands.NewSwitchCommand(repoProvider))
	root.AddCommand(commands.NewResolveCommand(toolProvider))
	root.AddCommand(commands.NewSchemaCommand(repoProvider))
	root.AddCommand(commands.NewValidateCommand(toolProvider))
	root.AddCommand(commands.NewDiffCommand(toolProvider))
	root.AddCommand(commands.NewHistoryCommand(toolProvider))
	root.AddCommand(commands.NewBranchesContainingCommand(toolProvider))
	root.AddCommand(commands.NewFilterCommand(toolProvider))
	root.AddCommand(commands.NewSearchCommand(toolProvider))
	root.AddCommand(commands.NewSearchExpandCommand(toolProvider))
	root.AddCommand(commands.NewContextCommand(toolProvider))
	root.AddCommand(commands.NewGraphCommand(toolProvider))
	root.AddCommand(commands.NewMergeCommand(repoProvider))
	root.AddCommand(commands.NewFsckCommand(fsckProvider))
	root.AddCommand(commands.NewGCCommand(repoProvider))
	root.AddCommand(commands.NewPruneCommand(repoProvider))
	root.AddCommand(commands.NewCherryPickCommand(repoProvider))
	workspaceCmd := commands.NewWorkspaceCommandDefault()
	workspaceCmd.AddCommand(commands.NewMigrateCommand(migrateProvider))
	root.AddCommand(workspaceCmd)
	root.AddCommand(commands.NewMigrateCommand(migrateProvider))
	root.AddCommand(commands.NewRemoteCommand(repoProvider))
	root.AddCommand(commands.NewCloneCommand())
	root.AddCommand(commands.NewPushCommand(repoProvider))
	root.AddCommand(commands.NewPullCommand(repoProvider))
	root.AddCommand(commands.NewAssetCommand(repoProvider))
	root.AddCommand(commands.NewVersionCommand())
	root.AddCommand(commands.NewMCPCommand(stateDirProvider))
	return root
}
