package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewPushCommand pushes verified local native commits and their packs to the
// repository's configured Rack remote over the native push protocol.
func NewPushCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var branchName, baseCommit string
	command := &cobra.Command{
		Use:   "push",
		Short: "Push local commits to the repository's configured Rack remote",
		Long: "Push builds a native pack from the local commits reachable from --branch that are not yet " +
			"known to Rack (per --base-commit), then sends it to the repository's configured Rack remote. " +
			"Push only supports linear, fast-forward history: it fails if any commit in range has more than " +
			"one parent, or if Rack reports the remote branch has moved past --base-commit.",
		Example:      "  spl push --branch main --base-commit <last-known-wire-commit-id>\n  spl push --branch main",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
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

			pack, err := repo.BuildPushPack(command.Context(), branchName, baseCommit)
			if err != nil {
				if errors.Is(err, repository.ErrNothingToPush) {
					return json.NewEncoder(command.OutOrStdout()).Encode(pushResult{
						Branch: branchName, Pushed: false, Message: "nothing to push: local branch is not ahead of --base-commit",
					})
				}
				return err
			}

			credential, err := resolvePushCredential(cfg)
			if err != nil {
				return fmt.Errorf("resolve rack credential: %w", err)
			}

			req := remote.PushRequest{
				Branch:       pack.Branch,
				BaseCommit:   pack.BaseCommit,
				TargetCommit: pack.TargetCommit,
				Commits:      toRemotePushCommits(pack.Commits),
				PackHash:     pack.PackHash,
				PackFormat:   uint32(pack.PackFormat),
				PackData:     pack.PackData,
			}
			result, err := remote.Push(command.Context(), remote.NewClient(), cfg, credential, req)
			if err != nil {
				var nffErr *remote.NonFastForwardError
				if errors.As(err, &nffErr) {
					return json.NewEncoder(command.OutOrStdout()).Encode(pushResult{
						Branch:     branchName,
						Pushed:     false,
						Rejected:   true,
						ActualHead: nffErr.ActualHead,
						Message:    nffErr.Guidance,
					})
				}
				return fmt.Errorf("push to rack: %w", err)
			}

			return json.NewEncoder(command.OutOrStdout()).Encode(pushResult{
				Branch:      result.Branch,
				Pushed:      true,
				HeadCommit:  result.HeadCommit,
				CommitsSent: len(pack.Commits),
			})
		},
	}
	command.Flags().StringVar(&branchName, "branch", "", "local branch to push")
	command.Flags().StringVar(&baseCommit, "base-commit", "", "last wire commit ID Rack is known to have for this branch; omit to push the entire history")
	_ = command.MarkFlagRequired("branch")
	return command
}

type pushResult struct {
	Branch      string `json:"branch"`
	Pushed      bool   `json:"pushed"`
	CommitsSent int    `json:"commitsSent,omitempty"`
	HeadCommit  string `json:"headCommit,omitempty"`
	Rejected    bool   `json:"rejected,omitempty"`
	ActualHead  string `json:"actualHead,omitempty"`
	Message     string `json:"message,omitempty"`
}

func toRemotePushCommits(commits []repository.PushCommitRecord) []remote.PushCommitRecord {
	converted := make([]remote.PushCommitRecord, len(commits))
	for i, commit := range commits {
		converted[i] = remote.PushCommitRecord{ID: commit.ID, Commit: commit.Commit}
	}
	return converted
}

// resolvePushCredential resolves a Rack credential from the OS keychain, the
// conventional environment variable, or an interactive terminal prompt, in
// that order. Unlike `spl remote show`'s best-effort probe, push is a
// state-changing operation: an unresolved credential is a hard error rather
// than falling back to an unauthenticated request.
func resolvePushCredential(cfg repository.RemoteConfig) (string, error) {
	credential, err := remote.ResolveCredential(cfg.RepoID, cfg.AuthMode, remote.ResolveOptions{
		Keychain: remote.KeyringStore{},
		Getenv:   os.Getenv,
		Prompt:   remote.TerminalPrompt(),
	})
	if err != nil {
		return "", err
	}
	return credential.Value, nil
}
