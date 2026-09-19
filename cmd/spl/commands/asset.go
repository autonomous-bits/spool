package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewAssetCommand creates the parent 'spl asset' command group.
func NewAssetCommand(opts ctxgit.Options) *cobra.Command {
	assetCmd := &cobra.Command{
		Use:          "asset",
		Short:        "Manage bound context-git reference assets",
		Long:         "Store reference documents in the bound context checkout (assets/) and link them as Asset graph nodes. Writes open a short-lived branch and pull request.",
		SilenceUsage: true,
	}
	assetCmd.AddCommand(newAssetAddCommand(opts))
	assetCmd.AddCommand(newAssetReadCommand(opts))
	return assetCmd
}

func newAssetAddCommand(opts ctxgit.Options) *cobra.Command {
	var filePath, title, nodeID, author, message string
	command := &cobra.Command{
		Use:          "add",
		Short:        "Ingest a reference document and commit an Asset node",
		Long:         "Ingest a file into the bound context checkout, write an Asset node, and open a short-lived branch + PR.",
		Example:      "  spl asset add --file docs/architecture.md --title 'Architecture notes'",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			session, err := startBoundSession(cmd, opts)
			if err != nil {
				return err
			}
			added, write, err := session.AddAsset(cmd.Context(), filePath, nodeID, title, author, message)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
				"node":        added.Node,
				"assetUri":    added.AssetURI,
				"hash":        added.Hash,
				"size":        added.Size,
				"mimeType":    added.MIMEType,
				"branch":      write.Branch,
				"commit":      write.Commit,
				"pullRequest": write.PR,
				"status":      added.Status,
			})
		},
	}
	command.Flags().StringVar(&filePath, "file", "", "path to the reference document to ingest")
	command.Flags().StringVar(&title, "title", "", "optional descriptive title for the Asset graph node")
	command.Flags().StringVar(&nodeID, "id", "", "optional explicit node ID for the Asset graph node")
	command.Flags().StringVar(&author, "author", "", "git author")
	command.Flags().StringVar(&message, "message", "", "commit/PR message")
	_ = command.MarkFlagRequired("file")
	return command
}

func newAssetReadCommand(opts ctxgit.Options) *cobra.Command {
	var locator, nodeID string
	command := &cobra.Command{
		Use:          "read [locator-or-node-id]",
		Short:        "Stream raw asset blob contents to standard output",
		Long:         "Stream raw asset bytes from the bound context checkout. Identify the asset by path, hash, or Asset node ID.",
		Example:      "  spl asset read --node demo-repo/notes",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
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
			session, err := startBoundSession(cmd, opts)
			if err != nil {
				return err
			}
			reader, _, _, err := session.ReadAsset(cmd.Context(), target)
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
	command.Flags().StringVar(&locator, "locator", "", "assets/ path, raw hash, or canonical locator")
	command.Flags().StringVar(&nodeID, "node", "", "graph node ID containing an asset reference")
	return command
}
