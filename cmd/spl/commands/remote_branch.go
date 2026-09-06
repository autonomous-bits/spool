package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// newRemoteBranchCommand creates the `spl remote branch` command group for
// managing remote (Rack) branch lifecycle: create, list, default-branch
// discovery, and protected deletion.
func newRemoteBranchCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	command := &cobra.Command{
		Use:          "branch",
		Short:        "Manage remote branches on the repository's configured Rack remote",
		Long:         "Create, list, and delete remote branches, and discover the remote's default branch. Every remote branch action is reported as JSON.",
		Example:      "  spl remote branch create feature --from-branch main\n  spl remote branch list\n  spl remote branch default\n  spl remote branch delete feature",
		SilenceUsage: true,
	}
	command.AddCommand(
		newRemoteBranchCreateCommand(repoProvider),
		newRemoteBranchListCommand(repoProvider),
		newRemoteBranchDefaultCommand(repoProvider),
		newRemoteBranchDeleteCommand(repoProvider),
	)
	return command
}

type remoteBranchResult struct {
	Name       string `json:"name"`
	HeadCommit string `json:"headCommit"`
}

func newRemoteBranchResult(result remote.BranchResult) remoteBranchResult {
	return remoteBranchResult{Name: result.Name, HeadCommit: result.HeadCommit}
}

// resolveRemoteBranchCredential resolves a Rack credential from the OS
// keychain, the conventional environment variable, or an interactive
// terminal prompt, in that order, matching resolvePushCredential: remote
// branch lifecycle actions are state-changing (or, for list/default, still
// require authenticated access), so an unresolved credential is a hard
// error rather than falling back to an unauthenticated request.
func resolveRemoteBranchCredential(cfg repository.RemoteConfig) (string, error) {
	return resolvePushCredential(cfg)
}

func remoteAndCredential(repo *repository.Repository) (repository.RemoteConfig, string, error) {
	cfg, ok, err := repo.Remote()
	if err != nil {
		return repository.RemoteConfig{}, "", err
	}
	if !ok {
		return repository.RemoteConfig{}, "", repository.ErrRemoteNotConfigured
	}
	credential, err := resolveRemoteBranchCredential(cfg)
	if err != nil {
		return repository.RemoteConfig{}, "", err
	}
	return cfg, credential, nil
}

func newRemoteBranchCreateCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var sourceBranch, sourceCommit string
	command := &cobra.Command{
		Use:          "create <name>",
		Short:        "Create a remote branch at an explicit source",
		Long:         "Create a remote branch on the repository's configured Rack remote from exactly one existing remote branch or commit, then record local remote-branch tracking metadata for it. The result is written as JSON to standard output.",
		Example:      "  spl remote branch create feature --from-branch main\n  spl remote branch create review --from-commit <commit-id>",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			cfg, credential, err := remoteAndCredential(repo)
			if err != nil {
				return err
			}
			result, err := remote.CreateBranch(command.Context(), remote.NewClient(), cfg, credential, remote.BranchCreateRequest{
				Name: args[0], SourceBranch: sourceBranch, SourceCommit: sourceCommit,
			})
			if err != nil {
				return writeRemoteErrorEnvelope(command, "create remote branch", err, credential)
			}
			if err := repo.SetRemoteBranchTracking(args[0], result.Name, result.HeadCommit); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(newRemoteBranchResult(result))
		},
	}
	command.Flags().StringVar(&sourceBranch, "from-branch", "", "existing remote branch to use as the source")
	command.Flags().StringVar(&sourceCommit, "from-commit", "", "existing remote commit to use as the source")
	return command
}

type remoteBranchListResult struct {
	Branches []remoteBranchListEntry `json:"branches"`
}

type remoteBranchListEntry struct {
	Name       string `json:"name"`
	HeadCommit string `json:"headCommit"`
	Default    bool   `json:"default,omitempty"`
}

func newRemoteBranchListCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "list",
		Short:        "List remote branches",
		Long:         "List the repository's configured Rack remote's branches as JSON.",
		Example:      "  spl remote branch list",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			cfg, credential, err := remoteAndCredential(repo)
			if err != nil {
				return err
			}
			result, err := remote.ListBranches(command.Context(), remote.NewClient(), cfg, credential)
			if err != nil {
				return writeRemoteErrorEnvelope(command, "list remote branches", err, credential)
			}
			entries := make([]remoteBranchListEntry, len(result.Branches))
			for i, branch := range result.Branches {
				entries[i] = remoteBranchListEntry{Name: branch.Name, HeadCommit: branch.HeadCommit, Default: branch.Default}
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(remoteBranchListResult{Branches: entries})
		},
	}
}

func newRemoteBranchDefaultCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "default",
		Short:        "Discover the remote's default branch",
		Long:         "Discover the repository's configured Rack remote's default (protected) branch and its current head commit, as JSON.",
		Example:      "  spl remote branch default",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			cfg, credential, err := remoteAndCredential(repo)
			if err != nil {
				return err
			}
			result, err := remote.DefaultBranch(command.Context(), remote.NewClient(), cfg, credential)
			if err != nil {
				return writeRemoteErrorEnvelope(command, "discover remote default branch", err, credential)
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(newRemoteBranchResult(result))
		},
	}
}

type remoteBranchDeleteResult struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

func newRemoteBranchDeleteCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "delete <name>",
		Short:        "Delete a remote branch",
		Long:         "Delete a remote branch from the repository's configured Rack remote. Rack rejects deleting its protected default branch; that rejection is reported as a JSON error envelope rather than a deletion.",
		Example:      "  spl remote branch delete feature",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			cfg, credential, err := remoteAndCredential(repo)
			if err != nil {
				return err
			}
			if err := remote.DeleteBranch(command.Context(), remote.NewClient(), cfg, credential, args[0]); err != nil {
				return writeRemoteErrorEnvelope(command, "delete remote branch", err, credential)
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(remoteBranchDeleteResult{Name: args[0], Deleted: true})
		},
	}
}
