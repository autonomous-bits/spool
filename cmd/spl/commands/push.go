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
	var reconcile bool
	command := &cobra.Command{
		Use:   "push",
		Short: "Push local commits to the repository's configured Rack remote",
		Long: "Push builds a native pack from the local commits reachable from --branch that are not yet " +
			"known to Rack (per --base-commit), then sends it to the repository's configured Rack remote. " +
			"Push only supports linear, fast-forward history: it fails if any commit in range has more than " +
			"one parent, or if Rack reports the remote branch has moved past --base-commit.\n\n" +
			"With --reconcile, a non-fast-forward rejection is handled automatically instead of just " +
			"reported: push fetches Rack's complete current history for --branch into a local " +
			"reconciliation branch, rebases --branch's independent local changes onto it with the graph " +
			"merge engine, and retries the push with the resulting fast-forward-eligible commit. If that " +
			"merge finds conflicts, push leaves both --branch and the reconciliation branch untouched and " +
			"reports the conflicts instead of retrying; resolve them with `spl merge preview/apply/conflicts/" +
			"resolve/finalize` against --branch and the reconciliation branch, then retry.",
		Example:      "  spl push --branch main --base-commit <last-known-wire-commit-id>\n  spl push --branch main\n  spl push --branch main --reconcile",
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

			credential, err := resolvePushCredential(cfg)
			if err != nil {
				return fmt.Errorf("resolve rack credential: %w", err)
			}

			attempt := func(base string) (remote.PushResult, repository.PushPack, error) {
				pack, err := repo.BuildPushPack(ctx, branchName, base)
				if err != nil {
					return remote.PushResult{}, repository.PushPack{}, err
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
				result, err := remote.Push(ctx, remote.NewClient(), cfg, credential, req)
				return result, pack, err
			}

			result, pack, err := attempt(baseCommit)
			reconciled := false
			if err != nil {
				if errors.Is(err, repository.ErrNothingToPush) {
					return json.NewEncoder(command.OutOrStdout()).Encode(pushResult{
						Branch: branchName, Pushed: false, Message: "nothing to push: local branch is not ahead of --base-commit",
					})
				}
				var nffErr *remote.NonFastForwardError
				if !errors.As(err, &nffErr) {
					return writeRemoteErrorEnvelope(command, "push to rack", err, credential)
				}
				if !reconcile {
					return json.NewEncoder(command.OutOrStdout()).Encode(pushResult{
						Branch:        branchName,
						Pushed:        false,
						Rejected:      true,
						ActualHead:    nffErr.ActualHead,
						Message:       nffErr.Guidance,
						CorrelationID: nffErr.CorrelationID,
					})
				}

				remoteHead, conflictResult, err := reconcilePush(ctx, repo, cfg, credential, branchName)
				if err != nil {
					return fmt.Errorf("reconcile %s: %w", branchName, err)
				}
				if conflictResult != nil {
					return json.NewEncoder(command.OutOrStdout()).Encode(conflictResult)
				}

				result, pack, err = attempt(remoteHead)
				if err != nil {
					var retryNFF *remote.NonFastForwardError
					if errors.As(err, &retryNFF) {
						return json.NewEncoder(command.OutOrStdout()).Encode(pushResult{
							Branch:        branchName,
							Pushed:        false,
							Rejected:      true,
							Reconciled:    true,
							ActualHead:    retryNFF.ActualHead,
							Message:       retryNFF.Guidance,
							CorrelationID: retryNFF.CorrelationID,
						})
					}
					return writeRemoteErrorEnvelope(command, "push to rack (post-reconciliation retry)", err, credential)
				}
				reconciled = true
			}

			// Establish or refresh local remote-branch tracking metadata so
			// a subsequent push or pull for this local branch knows which
			// remote branch it corresponds to and its last-known remote
			// head, without requiring a separate `spl remote branch
			// create` step for a branch that was pushed into existence.
			if err := repo.SetRemoteBranchTracking(branchName, result.Branch, result.HeadCommit); err != nil {
				return fmt.Errorf("update remote branch tracking: %w", err)
			}

			return json.NewEncoder(command.OutOrStdout()).Encode(pushResult{
				Branch:      result.Branch,
				Pushed:      true,
				HeadCommit:  result.HeadCommit,
				CommitsSent: len(pack.Commits),
				Reconciled:  reconciled,
			})
		},
	}
	command.Flags().StringVar(&branchName, "branch", "", "local branch to push")
	command.Flags().StringVar(&baseCommit, "base-commit", "", "last wire commit ID Rack is known to have for this branch; omit to push the entire history")
	command.Flags().BoolVar(&reconcile, "reconcile", false, "on a non-fast-forward rejection, fetch Rack's current history, rebase local changes onto it with the merge engine, and retry the push")
	_ = command.MarkFlagRequired("branch")
	return command
}

type pushResult struct {
	Branch        string `json:"branch"`
	Pushed        bool   `json:"pushed"`
	CommitsSent   int    `json:"commitsSent,omitempty"`
	HeadCommit    string `json:"headCommit,omitempty"`
	Rejected      bool   `json:"rejected,omitempty"`
	Reconciled    bool   `json:"reconciled,omitempty"`
	ActualHead    string `json:"actualHead,omitempty"`
	Message       string `json:"message,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
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
