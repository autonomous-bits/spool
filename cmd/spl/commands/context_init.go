package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewContextInitCommand creates `spl context init --remote`.
func NewContextInitCommand() *cobra.Command {
	var remote, solutionID, protectedBranch, repositoryID, author string
	command := &cobra.Command{
		Use:   "init",
		Short: "Bind this code repo to a solution context git remote and seed its layout",
		Long: "Write .spool/context.toml in this code repo, create the human-diffable context layout " +
			"(schema.toml, nodes/, edges/, assets/) on the remote when empty, and seed a CodeRepository " +
			"node from this bind. Repeat in each code repo that shares the same context remote. " +
			"MCP writes still open a short-lived branch and PR. Context sync is stock git clone/PR/history — " +
			"not Rack or `.spl`.",
		Example:      "  spl context init --remote https://github.com/org/solution-context.git",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := ctxgit.Init(command.Context(), ctxgit.InitRequest{
				Remote:          remote,
				SolutionID:      solutionID,
				ProtectedBranch: protectedBranch,
				RepositoryID:    repositoryID,
				Author:          author,
			}, ctxgit.GitRunner{})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&remote, "remote", "", "context git remote URL (stock git credentials)")
	command.Flags().StringVar(&solutionID, "solution-id", "", "solution identifier (defaults to the current directory name)")
	command.Flags().StringVar(&protectedBranch, "protected-branch", ctxgit.DefaultProtectedBranch, "protected integration branch")
	command.Flags().StringVar(&repositoryID, "repository-id", "", "code-repo namespace for node IDs (defaults to the current directory name)")
	command.Flags().StringVar(&author, "author", "spool-context <spool-context@localhost>", "git author for the layout commit")
	_ = command.MarkFlagRequired("remote")
	return command
}
