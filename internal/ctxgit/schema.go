package ctxgit

import (
	"context"
	"errors"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

// SchemaMigrateRequest replaces schema.toml and applies conforming graph mutations.
type SchemaMigrateRequest struct {
	SchemaTOML []byte
	Operations []repository.MutationOperation
	Author     string
	Message    string
}

// SchemaMigrateResult is the short-lived branch + PR from a bound schema write.
type SchemaMigrateResult struct {
	WriteResult
	Operations int `json:"operations"`
}

// MigrateSchema writes a target schema and optional graph mutations on a
// short-lived branch and opens a PR. Bound-only.
func (s *Session) MigrateSchema(ctx context.Context, request SchemaMigrateRequest) (SchemaMigrateResult, error) {
	if s == nil || !s.Bound() {
		return SchemaMigrateResult{}, UnboundError()
	}
	if len(request.SchemaTOML) == 0 {
		return SchemaMigrateResult{}, repository.ErrInvalidSchemaTOML
	}
	if _, err := repository.DecodeSchemaTOML(request.SchemaTOML); err != nil {
		return SchemaMigrateResult{}, err
	}
	if err := s.syncProtected(ctx); err != nil {
		return SchemaMigrateResult{}, err
	}
	base, err := LoadGraph(s.CheckoutDir)
	if err != nil {
		return SchemaMigrateResult{}, err
	}
	s.graph = base
	working := cloneGraph(base)
	working.SchemaTOML = append([]byte(nil), request.SchemaTOML...)
	after := working
	var overlaps []Overlap
	ops := request.Operations
	if len(ops) > 0 {
		namespaced := namespaceOperations(s.Bind.RepositoryID, ops)
		normalized, err := normalizeOperations(namespaced)
		if err != nil {
			return SchemaMigrateResult{}, err
		}
		if err := validateBatch(working, normalized); err != nil {
			return SchemaMigrateResult{}, err
		}
		after, overlaps, err = applyOperations(working, normalized)
		if err != nil {
			return SchemaMigrateResult{}, err
		}
		after.SchemaTOML = append([]byte(nil), request.SchemaTOML...)
		schema, err := schemaSnapshot(after)
		if err != nil {
			return SchemaMigrateResult{}, err
		}
		if err := repository.ValidateSchemaSnapshot(schema, after.Nodes, after.Edges); err != nil {
			return SchemaMigrateResult{}, err
		}
		ops = normalized
	} else {
		schema, err := schemaSnapshot(after)
		if err != nil {
			return SchemaMigrateResult{}, err
		}
		if err := repository.ValidateSchemaSnapshot(schema, after.Nodes, after.Edges); err != nil {
			return SchemaMigrateResult{}, err
		}
	}
	message := strings.TrimSpace(request.Message)
	if message == "" {
		message = "Migrate context schema"
	}
	write, err := s.commitGraphDiff(ctx, base, after, overlaps, request.Author, message, s.Bind.ProtectedBranch)
	if err != nil {
		return SchemaMigrateResult{}, err
	}
	return SchemaMigrateResult{WriteResult: write, Operations: len(ops)}, nil
}

// ValidateSchema checks the bound checkout graph against schema.toml.
func (s *Session) ValidateSchema() (resolve.SchemaValidationResult, error) {
	if s == nil || !s.Bound() {
		return resolve.SchemaValidationResult{}, UnboundError()
	}
	schema, err := schemaSnapshot(s.graph)
	if err != nil {
		return resolve.SchemaValidationResult{}, err
	}
	snapshot, projection := s.QuerySnapshot(s.Bind.ProtectedBranch)
	result := resolve.SchemaValidationResult{
		Snapshot: snapshot,
		Projection: projection,
		Schema: resolve.SchemaMetadata{
			Root:       "schema.toml",
			Version:    schema.Version,
			Permissive: schema.Permissive,
		},
		Valid:      true,
		Violations: []repository.SchemaViolation{},
	}
	if err := repository.ValidateSchemaSnapshot(schema, s.graph.Nodes, s.graph.Edges); err != nil {
		var validation *repository.SchemaValidationError
		if errors.As(err, &validation) {
			result.Valid = false
			result.Violations = validation.Violations
			return result, nil
		}
		return resolve.SchemaValidationResult{}, err
	}
	return result, nil
}

// AddAsset ingests a file, stages an Asset node, and commits via short-lived branch + PR.
func (s *Session) AddAsset(ctx context.Context, filePath, id, title, author, message string) (repository.AssetAddResult, WriteResult, error) {
	added, err := s.StageAsset(filePath, id, title)
	if err != nil {
		return repository.AssetAddResult{}, WriteResult{}, err
	}
	if strings.TrimSpace(message) == "" {
		message = "Add context asset"
	}
	write, err := s.Commit(ctx, author, message)
	if err != nil {
		return repository.AssetAddResult{}, WriteResult{}, err
	}
	added.Staged = false
	added.Status = "committed"
	added.Branch = write.Branch
	return added, write, nil
}
