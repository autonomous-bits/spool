package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewAssetCommand creates the parent 'spl asset' command group.
func NewAssetCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	assetCmd := &cobra.Command{
		Use:          "asset",
		Short:        "Manage contextual reference assets in content-addressable storage",
		Long:         "Manage content-addressable reference assets (specs, diagrams, research artifacts) and link them to knowledge graph entities.",
		SilenceUsage: true,
	}

	assetCmd.AddCommand(newAssetAddCommand(repoProvider))
	assetCmd.AddCommand(newAssetReadCommand(repoProvider))

	return assetCmd
}

func newAssetAddCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var branch, filePath, title, nodeID string
	command := &cobra.Command{
		Use:   "add",
		Short: "Ingest a reference document and stage a corresponding Asset node",
		Long: "Ingest a file into local content-addressable storage (.spl/assets/loose), " +
			"compute its BLAKE3-256 hash and MIME type, and stage an Asset node into the branch mutation set. " +
			"The command writes machine-readable JSON to standard output matching contract-asset-add-cli-payload.",
		Example: "  spl asset add --branch main --file docs/architecture.md\n" +
			"  spl asset add --branch main --file diagram.svg --title 'System Topology' --id asset-topology",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			if filePath == "" {
				return errors.New("file path is required (--file)")
			}
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				return fmt.Errorf("stat asset file: %w", err)
			}
			if fileInfo.IsDir() {
				return fmt.Errorf("path %q is a directory, expected a regular file", filePath)
			}

			repo, err := repoProvider()
			if err != nil {
				return err
			}

			result, err := repo.StageAsset(cmd.Context(), repository.AssetAddRequest{
				Branch:   branch,
				FilePath: filePath,
				Title:    title,
				ID:       nodeID,
			})
			if err != nil {
				return err
			}

			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		},
	}

	command.Flags().StringVar(&branch, "branch", "", "branch on which to stage the asset node")
	command.Flags().StringVar(&filePath, "file", "", "path to the reference document to ingest")
	command.Flags().StringVar(&title, "title", "", "optional descriptive title for the Asset graph node")
	command.Flags().StringVar(&nodeID, "id", "", "optional explicit node ID for the Asset graph node")

	_ = command.MarkFlagRequired("branch")
	_ = command.MarkFlagRequired("file")

	return command
}

func newAssetReadCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var locator, nodeID, branch string
	command := &cobra.Command{
		Use:   "read [locator-or-node-id]",
		Short: "Stream raw asset blob contents to standard output",
		Long: "Stream raw, unencoded asset bytes directly to standard output with constant memory overhead. " +
			"The asset can be identified either by its canonical locator URI (spool://assets/{hash}), " +
			"its raw BLAKE3 hash, or an Asset node ID in the branch graph.",
		Example: "  spl asset read --locator spool://assets/0123456789abcdef...\n" +
			"  spl asset read --node asset-topology --branch main\n" +
			"  spl asset read spool://assets/0123456789abcdef...",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmd.Context().Err(); err != nil {
				return err
			}

			target := locator
			if target == "" {
				target = nodeID
			}
			if target == "" && len(args) > 0 {
				target = args[0]
			}
			if target == "" {
				return errors.New("either --locator, --node, or a positional locator/node-id argument is required")
			}

			repo, err := repoProvider()
			if err != nil {
				return err
			}

			reader, _, _, err := repo.ReadAsset(cmd.Context(), branch, target)
			if err != nil {
				return err
			}
			defer func() { _ = reader.Close() }()

			buf := make([]byte, 32*1024)
			if _, err := io.CopyBuffer(cmd.OutOrStdout(), reader, buf); err != nil {
				return fmt.Errorf("stream asset contents: %w", err)
			}
			return nil
		},
	}

	command.Flags().StringVar(&locator, "locator", "", "canonical asset URI (spool://assets/{hash}) or raw BLAKE3 hash")
	command.Flags().StringVar(&nodeID, "node", "", "graph node ID containing an asset reference")
	command.Flags().StringVar(&branch, "branch", "", "branch to resolve node ID from (defaults to active branch)")

	return command
}
