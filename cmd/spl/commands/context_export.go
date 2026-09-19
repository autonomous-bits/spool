package commands

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewContextExportCommand creates `spl context export` (alias `migrate-once`).
func NewContextExportCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var branch, author, message string
	command := &cobra.Command{
		Use:     "export",
		Aliases: []string{"migrate-once"},
		Short:   "Best-effort one-shot .spl export to the bound context git remote",
		Long: "Map the selected local .spl branch snapshot onto the bound solution context git remote " +
			"as one mutation batch, one commit on a short-lived branch, and one pull request. " +
			"This is migrate-once, not sync. Kept: nodes, edges, schema, assets (Git LFS at or above 512 KiB). " +
			"Dropped: packs, Rack remotes, reflogs, merge leases, projections. Lossy is OK. " +
			"Re-run is overwrite-at-own-risk. Requires .spool/context.toml. " +
			"Does not configure Rack remotes and does not write .spl. " +
			"If the commit or PR fails after files were written, clean up the short-lived branch manually.",
		Example: "  spl context export --branch main\n" +
			"  spl context migrate-once --branch main --message \"One-shot export\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if repoProvider == nil {
				return errors.New("repository provider is not configured")
			}
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			session, err := ctxgit.Start(command.Context(), ctxgit.Options{WorkspaceDir: wd})
			if err != nil {
				return err
			}
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
	command.Flags().StringVar(&branch, "branch", "", "local .spl branch to export (defaults to the active branch)")
	command.Flags().StringVar(&author, "author", "spool-export <spool-export@localhost>", "git author for the export commit")
	command.Flags().StringVar(&message, "message", "", "commit/PR title (defaults to a migrate-once overwrite-at-own-risk message)")
	return command
}
