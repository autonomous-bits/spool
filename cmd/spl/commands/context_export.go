package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewContextExportCommand creates `spl context export` (alias `migrate-once`).
func NewContextExportCommand(opts ctxgit.Options) *cobra.Command {
	var from, branch, author, message string
	command := &cobra.Command{
		Use:     "export",
		Aliases: []string{"migrate-once"},
		Short:   "Best-effort one-shot .spl export to the bound context git remote",
		Long: "Read leftover .spl internally and map the selected branch snapshot onto the bound " +
			"solution context git remote as one mutation batch, one commit on a short-lived branch, " +
			"and one pull request. This is migrate-once, not sync. Kept: nodes, edges, schema, assets. " +
			"Dropped: packs, Rack remotes, reflogs, merge leases, projections. Lossy is OK. " +
			"Re-run is overwrite-at-own-risk. Requires .spool/context.toml. " +
			"Does not configure Rack remotes and does not write .spl.",
		Example: "  spl context export\n" +
			"  spl context migrate-once --branch main --message \"One-shot export\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			workspace := opts.WorkspaceDir
			if workspace == "" {
				workspace, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			repo, err := openLeftoverSpl(from, workspace)
			if err != nil {
				return err
			}
			defer func() { _ = repo.Close() }()
			result, err := session.Export(command.Context(), ctxgit.ExportRequest{
				Repo:    repo,
				Branch:  branch,
				Author:  author,
				Message: message,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&from, "from", "", "leftover .spl state directory (defaults to <workspace>/.spl)")
	command.Flags().StringVar(&branch, "branch", "", "legacy .spl branch to export (defaults to that repository's default branch)")
	command.Flags().StringVar(&author, "author", "spool-export <spool-export@localhost>", "git author for the export commit")
	command.Flags().StringVar(&message, "message", "", "commit/PR title (defaults to a migrate-once overwrite-at-own-risk message)")
	return command
}

func openLeftoverSpl(from, workspace string) (*repository.Repository, error) {
	stateDir := from
	if stateDir == "" {
		stateDir = filepath.Join(workspace, ".spl")
	}
	repo, err := repository.OpenRepository(stateDir)
	if err != nil {
		return nil, fmt.Errorf("open leftover .spl at %s: %w", stateDir, err)
	}
	return repo, nil
}
