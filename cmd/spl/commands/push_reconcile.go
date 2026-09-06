package commands

import (
	"context"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
)

// pushReconcileConflictResult is the JSON envelope `spl push --reconcile`
// reports when the graph merge engine finds conflicts between the local
// branch's independent changes and Rack's current history. Neither the
// local branch nor the reconciliation branch is mutated in this case: both
// histories are retained so the conflicts can be resolved with the existing
// `spl merge` commands before retrying.
type pushReconcileConflictResult struct {
	Branch               string                     `json:"branch"`
	Pushed               bool                       `json:"pushed"`
	Reconciled           bool                       `json:"reconciled"`
	Conflicted           bool                       `json:"conflicted"`
	ReconciliationBranch string                     `json:"reconciliationBranch"`
	Message              string                     `json:"message"`
	Conflicts            []repository.MergeConflict `json:"conflicts"`
}

// reconcilePush fetches Rack's complete current history for branchName,
// installs it onto branchName's dedicated reconciliation branch, and
// rebases branchName's independent local changes onto it. On a clean
// result it returns the Rack wire commit ID the retried push should use as
// its --base-commit; on conflicts it returns a populated
// pushReconcileConflictResult (and an empty remoteHead) for the caller to
// report instead of retrying.
func reconcilePush(ctx context.Context, repo *repository.Repository, cfg repository.RemoteConfig, credential, branchName string) (remoteHead string, conflict *pushReconcileConflictResult, err error) {
	pulled, err := remote.Pull(ctx, remote.NewClient(), cfg, credential, branchName, "")
	if err != nil {
		return "", nil, fmt.Errorf("fetch rack's current history: %w", err)
	}
	if pulled.UpToDate {
		return "", nil, errors.New("rack reported the branch is up to date immediately after rejecting a push as non-fast-forward")
	}

	reconciliationBranch := repository.ReconciliationBranchName(branchName)
	if _, err := repo.InstallReconciliationPack(ctx, reconciliationBranch, pulled.Packs, pulled.HeadCommit); err != nil {
		return "", nil, fmt.Errorf("install rack's current history: %w", err)
	}

	message := fmt.Sprintf("reconcile onto rack head %s", pulled.HeadCommit)
	result, err := repo.ReconcileBranch(branchName, reconciliationBranch, "", message)
	if err != nil {
		if errors.Is(err, repository.ErrMergeConflicted) {
			return "", &pushReconcileConflictResult{
				Branch:               branchName,
				Pushed:               false,
				Reconciled:           false,
				Conflicted:           true,
				ReconciliationBranch: reconciliationBranch,
				Message: fmt.Sprintf(
					"reconciliation found conflicts between %s and rack's current history; resolve them with "+
						"`spl merge preview --source %s --target %s` (then `spl merge apply`/`resolve`/`finalize`), "+
						"and retry `spl push --branch %s --reconcile`",
					branchName, branchName, reconciliationBranch, branchName,
				),
				Conflicts: result.Preview.Conflicts,
			}, nil
		}
		return "", nil, fmt.Errorf("rebase %s onto rack's current history: %w", branchName, err)
	}
	return pulled.HeadCommit, nil, nil
}
