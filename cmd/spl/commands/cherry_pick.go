package commands

import (
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewCherryPickCommand creates the cherry-pick command for selectively transplanting commits.
func NewCherryPickCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var (
		commit       string
		targetBranch string
		dryRun       bool
		author       string
		message      string
	)
	command := &cobra.Command{
		Use:          "cherry-pick",
		Short:        "Cherry-pick a commit onto a target branch",
		Long:         "Apply the exact graph delta of a historical commit onto a target branch without incorporating unrelated branch history. The result is emitted as machine-readable JSON.",
		Example:      "  spl cherry-pick --commit <commit-id> --target-branch main\n  spl cherry-pick --commit <commit-id> --target-branch main --dry-run\n  spl cherry-pick --commit <commit-id> --target-branch main --author \"Alice <alice@example.com>\" --message \"Cherry-pick fix\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := command.Context().Err(); err != nil {
				return err
			}
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			result, cherryPickErr := repo.CherryPick(repository.CherryPickRequest{
				Commit:       commit,
				TargetBranch: targetBranch,
				DryRun:       dryRun,
				Author:       author,
				Message:      message,
			})
			if cherryPickErr != nil {
				var warning *repository.CherryPickCommittedWithWarningError
				if !errors.As(cherryPickErr, &warning) && !errors.Is(cherryPickErr, repository.ErrCherryPickConflicts) {
					return cherryPickErr
				}
			}
			if err := json.NewEncoder(command.OutOrStdout()).Encode(result); err != nil {
				return err
			}
			return cherryPickErr
		},
	}
	command.Flags().StringVar(&commit, "commit", "", "source commit hash whose graph delta to transplant")
	command.Flags().StringVar(&targetBranch, "target-branch", "", "branch onto which to apply the transplanted delta")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "simulate cherry-picking and report changes without modifying target branch")
	command.Flags().StringVar(&author, "author", "", "override commit author")
	command.Flags().StringVar(&message, "message", "", "override commit message")
	_ = command.MarkFlagRequired("commit")
	_ = command.MarkFlagRequired("target-branch")
	return command
}
