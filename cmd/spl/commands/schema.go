package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewSchemaCommand creates schema authoring and migration commands.
func NewSchemaCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	command := &cobra.Command{
		Use:          "schema",
		Short:        "Author and migrate graph schemas",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
	}
	command.AddCommand(NewSchemaMigrateCommand(repoProvider))
	return command
}

// NewSchemaMigrateCommand creates the command that atomically stages a schema
// migration and its graph mutation batch.
func NewSchemaMigrateCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var branchName, schemaPath, batchPath string
	command := &cobra.Command{
		Use:          "migrate",
		Short:        "Stage a schema migration and conforming graph mutations",
		Long:         "Read a target schema from TOML and a complete JSON mutation batch, then atomically replace the branch's staged set after validating the candidate graph against the target schema. Commit the staged set to install the schema and graph changes together.",
		Example:      "  spl schema migrate --branch main --schema people.toml --batch people-mutations.json",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := command.Context().Err(); err != nil {
				return err
			}
			schemaTOML, err := os.ReadFile(schemaPath)
			if err != nil {
				return fmt.Errorf("read schema TOML: %w", err)
			}
			data, err := os.ReadFile(batchPath)
			if err != nil {
				return fmt.Errorf("read mutation batch: %w", err)
			}
			var operations []repository.MutationOperation
			if err := json.Unmarshal(data, &operations); err != nil {
				return fmt.Errorf("decode mutation batch: %w", err)
			}
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			result, err := repo.StageSchemaMigration(repository.SchemaMigrationRequest{
				Branch: branchName, SchemaTOML: schemaTOML, Operations: operations,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&branchName, "branch", "", "branch on which to stage the migration")
	command.Flags().StringVar(&schemaPath, "schema", "", "path to the target TOML schema")
	command.Flags().StringVar(&batchPath, "batch", "", "path to a JSON mutation-operation array")
	_ = command.MarkFlagRequired("branch")
	_ = command.MarkFlagRequired("schema")
	_ = command.MarkFlagRequired("batch")
	return command
}
