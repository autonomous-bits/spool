package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewSchemaCommand creates schema authoring commands for bound context git.
func NewSchemaCommand(opts ctxgit.Options) *cobra.Command {
	command := &cobra.Command{
		Use:          "schema",
		Short:        "Author and migrate bound context schemas",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
	}
	command.AddCommand(NewSchemaMigrateCommand(opts))
	return command
}

// NewSchemaMigrateCommand writes schema.toml and conforming mutations via branch+PR.
func NewSchemaMigrateCommand(opts ctxgit.Options) *cobra.Command {
	var schemaPath, batchPath, author, message string
	command := &cobra.Command{
		Use:          "migrate",
		Short:        "Write a schema migration and conforming graph mutations",
		Long:         "Read a target schema from TOML and a JSON mutation batch, validate the candidate graph, and open a short-lived branch + PR on the bound context remote.",
		Example:      "  spl schema migrate --schema people.toml --batch people-mutations.json",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			schemaTOML, err := os.ReadFile(schemaPath)
			if err != nil {
				return fmt.Errorf("read schema TOML: %w", err)
			}
			var operations []repository.MutationOperation
			if batchPath != "" {
				data, err := os.ReadFile(batchPath)
				if err != nil {
					return fmt.Errorf("read mutation batch: %w", err)
				}
				if err := json.Unmarshal(data, &operations); err != nil {
					return fmt.Errorf("decode mutation batch: %w", err)
				}
			}
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.MigrateSchema(command.Context(), ctxgit.SchemaMigrateRequest{
				SchemaTOML: schemaTOML,
				Operations: operations,
				Author:     author,
				Message:    message,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&schemaPath, "schema", "", "path to the target TOML schema")
	command.Flags().StringVar(&batchPath, "batch", "", "path to a JSON mutation-operation array")
	command.Flags().StringVar(&author, "author", "", "git author")
	command.Flags().StringVar(&message, "message", "", "commit/PR message")
	_ = command.MarkFlagRequired("schema")
	return command
}
