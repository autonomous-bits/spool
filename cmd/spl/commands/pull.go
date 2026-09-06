package commands

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewPullCommand fetches new commits for a branch from the repository's
// configured Rack remote over the native pull protocol and installs them
// onto local history.
func NewPullCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var branchName string
	command := &cobra.Command{
		Use:   "pull",
		Short: "Pull new commits for a branch from the repository's configured Rack remote",
		Long: "Pull asks Rack what it has for --branch beyond the local branch's own recomputed wire commit " +
			"ID, then downloads and installs any new commits as a fast-forward extension of local history. " +
			"Pull only supports fast-forward installs: it fails if the local branch has no history in " +
			"common with what Rack reports (a from-scratch bootstrap), and reports divergence rather than " +
			"attempting a merge if Rack's branch head is not a descendant of the local branch.",
		Example:      "  spl pull --branch main",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			ctx := command.Context()
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			cfg, ok, err := repo.Remote()
			if err != nil {
				return err
			}
			if !ok {
				return repository.ErrRemoteNotConfigured
			}

			// The local branch's current recomputed wire commit ID is what
			// Rack already knows we have; BuildPushPack's TargetCommit is
			// always the branch head's wire ID regardless of base, so it
			// doubles as a cheap way to compute this without a separate
			// remote-tracking state.
			local, err := repo.BuildPushPack(ctx, branchName, "")
			if err != nil {
				return fmt.Errorf("determine local branch head: %w", err)
			}

			credential, err := resolvePushCredential(cfg)
			if err != nil {
				return fmt.Errorf("resolve rack credential: %w", err)
			}

			pulled, err := remote.Pull(ctx, remote.NewClient(), cfg, credential, branchName, local.TargetCommit)
			if err != nil {
				var diverged *remote.DivergedError
				if errors.As(err, &diverged) {
					return json.NewEncoder(command.OutOrStdout()).Encode(pullResult{
						Branch:     branchName,
						Pulled:     false,
						Diverged:   true,
						ActualHead: diverged.ActualHead,
						Message:    diverged.Guidance,
					})
				}
				return fmt.Errorf("pull from rack: %w", err)
			}
			if pulled.UpToDate {
				return json.NewEncoder(command.OutOrStdout()).Encode(pullResult{
					Branch:     branchName,
					Pulled:     false,
					UpToDate:   true,
					HeadCommit: pulled.HeadCommit,
					Message:    "up to date: local branch already has Rack's reported head commit",
				})
			}

			installed, err := repo.InstallPullPack(ctx, branchName, pulled.Packs, pulled.HeadCommit)
			if err != nil {
				return fmt.Errorf("install pulled pack: %w", err)
			}

			return json.NewEncoder(command.OutOrStdout()).Encode(pullResult{
				Branch:           installed.Branch,
				Pulled:           true,
				CommitsInstalled: installed.CommitsInstalled,
				HeadCommit:       pulled.HeadCommit,
			})
		},
	}
	command.Flags().StringVar(&branchName, "branch", "", "local branch to pull")
	_ = command.MarkFlagRequired("branch")
	return command
}

type pullResult struct {
	Branch           string `json:"branch"`
	Pulled           bool   `json:"pulled"`
	UpToDate         bool   `json:"upToDate,omitempty"`
	Diverged         bool   `json:"diverged,omitempty"`
	ActualHead       string `json:"actualHead,omitempty"`
	CommitsInstalled int    `json:"commitsInstalled,omitempty"`
	HeadCommit       string `json:"headCommit,omitempty"`
	Message          string `json:"message,omitempty"`
}
