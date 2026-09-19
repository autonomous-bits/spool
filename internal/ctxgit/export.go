package ctxgit

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
)

const (
	exportEntrypoint     = "export/migrate-once"
	defaultExportMessage = "One-shot .spl export to context git (migrate-once, overwrite-at-own-risk)"
)

// ExportRequest is a best-effort one-shot .spl → context git migration.
// It is not sync, not dual-write, and does not configure Rack remotes.
type ExportRequest struct {
	Repo    *repository.Repository
	Branch  string
	Author  string
	Message string
}

// ExportResult is the JSON payload for `spl context export` / `spl_context_export`.
type ExportResult struct {
	WriteResult
	Kept                []string `json:"kept"`
	Skipped             []string `json:"skipped"`
	Lossy               bool     `json:"lossy"`
	OverwriteAtOwnRisk  bool     `json:"overwriteAtOwnRisk"`
	Entrypoint          string   `json:"entrypoint"`
	Note                string   `json:"note"`
	RackRemoteUnchanged bool     `json:"rackRemoteUnchanged"`
}

const exportNote = "Best-effort one-shot .spl → context git export (migrate-once). Lossy is OK. Re-run is overwrite-at-own-risk, not continuous migration. Not sync. `.spl` is not written; Rack remotes are not configured as a side effect. There is no dual-write window."

// Export maps the selected .spl branch snapshot onto the bound context git
// remote as one mutation batch, one commit, and one PR. Packs, Rack remotes,
// reflogs, merge leases, and projections are skipped and must not land in the
// context tree.
func (s *Session) Export(ctx context.Context, req ExportRequest) (ExportResult, error) {
	if s == nil || !s.Bound() {
		return ExportResult{}, UnboundError()
	}
	if req.Repo == nil {
		return ExportResult{}, fmt.Errorf("repository is required")
	}

	beforeRemote, beforeOK, err := req.Repo.Remote()
	if err != nil {
		return ExportResult{}, err
	}

	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		init, err := req.Repo.Initialization()
		if err != nil {
			return ExportResult{}, err
		}
		branch = init.ActiveBranch
	}
	commitID, err := req.Repo.PinBranchContext(ctx, branch)
	if err != nil {
		return ExportResult{}, err
	}
	nodes, err := req.Repo.PinnedNodesContext(ctx, commitID)
	if err != nil {
		return ExportResult{}, err
	}
	edges, err := req.Repo.PinnedEdgesContext(ctx, commitID)
	if err != nil {
		return ExportResult{}, err
	}
	schemaRes, err := req.Repo.ValidatePinnedSchema(commitID)
	if err != nil {
		return ExportResult{}, err
	}
	schemaTOML, err := repository.EncodeSchemaTOML(schemaRes.Schema)
	if err != nil {
		return ExportResult{}, fmt.Errorf("encode schema: %w", err)
	}

	ops, assets, assetWarnings, err := exportGraphOperations(ctx, req.Repo, branch, nodes, edges, s.Bind.LFSThreshold())
	if err != nil {
		return ExportResult{}, err
	}

	skipped := skipInventory(req.Repo)
	kept := []string{
		fmt.Sprintf("nodes (%d)", len(nodes)),
		fmt.Sprintf("edges (%d)", len(edges)),
		"schema (schema.toml)",
		fmt.Sprintf("assets (%d)", len(assets)),
	}
	sort.Strings(kept)
	sort.Strings(skipped)

	summary := &ExportSummary{
		Kept:    append([]string(nil), kept...),
		Skipped: append([]string(nil), skipped...),
		Lossy:   true,
		Note:    exportNote,
	}

	if _, err := s.stageExport(ops, assets, schemaTOML, summary); err != nil {
		return ExportResult{}, err
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		message = defaultExportMessage
	}
	written, err := s.Commit(ctx, req.Author, message)
	if err != nil {
		return ExportResult{}, fmt.Errorf("export commit/PR failed (partial tree may remain on the short-lived branch; clean up manually): %w", err)
	}
	written.Warnings = append(written.Warnings, assetWarnings...)

	afterRemote, afterOK, err := req.Repo.Remote()
	if err != nil {
		return ExportResult{}, err
	}
	if beforeOK != afterOK || (beforeOK && (beforeRemote.Endpoint != afterRemote.Endpoint || beforeRemote.AuthMode != afterRemote.AuthMode)) {
		return ExportResult{}, fmt.Errorf("export must not configure Rack remotes as a side effect")
	}

	return ExportResult{
		WriteResult:         written,
		Kept:                kept,
		Skipped:             skipped,
		Lossy:               true,
		OverwriteAtOwnRisk:  true,
		Entrypoint:          exportEntrypoint,
		Note:                exportNote,
		RackRemoteUnchanged: true,
	}, nil
}

func (s *Session) stageExport(operations []repository.MutationOperation, assets []StagedAsset, schemaTOML []byte, summary *ExportSummary) (StageResult, error) {
	if len(schemaTOML) > 0 {
		s.graph.SchemaTOML = append([]byte(nil), schemaTOML...)
	}
	result, err := s.Stage(operations)
	if err != nil {
		return StageResult{}, err
	}
	s.staged.Assets = append([]StagedAsset(nil), assets...)
	s.staged.SchemaTOML = append([]byte(nil), schemaTOML...)
	s.staged.Export = summary
	return result, nil
}

func exportGraphOperations(ctx context.Context, repo *repository.Repository, branch string, nodes []repository.Node, edges []repository.Edge, lfsThreshold int64) ([]repository.MutationOperation, []StagedAsset, []string, error) {
	ops := make([]repository.MutationOperation, 0, len(nodes)+len(edges))
	var assets []StagedAsset
	var warnings []string
	for _, node := range nodes {
		cloned := node.Clone()
		if isAssetNode(cloned) {
			staged, warn, err := exportAssetBlob(ctx, repo, branch, cloned, lfsThreshold)
			if err != nil {
				return nil, nil, nil, err
			}
			if warn != "" {
				warnings = append(warnings, warn)
			}
			if staged.Hash != "" {
				assets = append(assets, staged)
				cloned.Properties = withAssetProperties(cloned.Properties, staged)
			}
		}
		ops = append(ops, repository.MutationOperation{
			Action:     "add",
			Entity:     "node",
			ID:         cloned.ID,
			Title:      cloned.Title,
			Labels:     cloned.Labels,
			Properties: cloned.Properties,
		})
	}
	for _, edge := range edges {
		cloned := edge.Clone()
		ops = append(ops, repository.MutationOperation{
			Action:     "add",
			Entity:     "edge",
			ID:         cloned.ID,
			Source:     cloned.Source,
			Target:     cloned.Target,
			Type:       cloned.Type,
			Properties: cloned.Properties,
		})
	}
	return ops, assets, warnings, nil
}

func isAssetNode(node repository.Node) bool {
	for _, label := range node.Labels {
		if label == "Asset" {
			return true
		}
	}
	if v, ok := node.Properties["assetUri"]; ok && v.Kind == repository.PropertyString && v.String != "" {
		return true
	}
	return false
}

func exportAssetBlob(ctx context.Context, repo *repository.Repository, branch string, node repository.Node, lfsThreshold int64) (StagedAsset, string, error) {
	locator := node.ID
	if v, ok := node.Properties["assetUri"]; ok && v.Kind == repository.PropertyString && v.String != "" {
		locator = v.String
	}
	reader, _, meta, err := repo.ReadAsset(ctx, branch, locator)
	if err != nil {
		return StagedAsset{}, fmt.Sprintf("skipped asset blob for node %s: %v", node.ID, err), nil
	}
	defer func() { _ = reader.Close() }()
	data, err := io.ReadAll(reader)
	if err != nil {
		return StagedAsset{}, "", fmt.Errorf("read asset %s: %w", node.ID, err)
	}
	filename := meta.OriginalFilename
	if filename == "" {
		if v, ok := node.Properties["originalFilename"]; ok && v.Kind == repository.PropertyString {
			filename = v.String
		}
	}
	if filename == "" {
		filename = "asset.bin"
	}
	staged, err := ingestAssetBytes(filename, data, lfsThreshold)
	if err != nil {
		return StagedAsset{}, "", err
	}
	return staged, staged.Warn, nil
}

func withAssetProperties(props map[string]repository.PropertyValue, staged StagedAsset) map[string]repository.PropertyValue {
	out := make(map[string]repository.PropertyValue, len(props)+4)
	for k, v := range props {
		out[k] = v
	}
	out["assetPath"] = repository.StringPropertyValue(staged.RelPath)
	out["hash"] = repository.StringPropertyValue(staged.Hash)
	out["byteSize"] = repository.IntegerPropertyValue(staged.Size)
	if staged.MIMEType != "" {
		out["mimeType"] = repository.StringPropertyValue(staged.MIMEType)
	}
	if staged.Filename != "" {
		out["originalFilename"] = repository.StringPropertyValue(staged.Filename)
	}
	return out
}

func skipInventory(repo *repository.Repository) []string {
	stateDir := repo.StateDir()
	skipped := []string{
		skipDir("packs", filepath.Join(stateDir, "objects", "pack"), "not copied"),
		skipRackRemote(repo),
		skipDir("reflogs", filepath.Join(stateDir, "logs"), "not copied"),
		skipDir("merge leases", filepath.Join(stateDir, "merge"), "not copied"),
		skipProjection(stateDir),
	}
	return skipped
}

func skipDir(kind, dir, reason string) string {
	count := countFiles(dir)
	return fmt.Sprintf("%s (%d files; %s)", kind, count, reason)
}

func skipProjection(stateDir string) string {
	count := 0
	for _, name := range []string{"graph.db", "graph.db-wal", "graph.db-shm", "projection.db"} {
		if _, err := os.Stat(filepath.Join(stateDir, name)); err == nil {
			count++
		}
	}
	return fmt.Sprintf("projections (%d files; not copied; rebuild-on-start)", count)
}

func skipRackRemote(repo *repository.Repository) string {
	cfg, ok, err := repo.Remote()
	if err != nil || !ok {
		return "Rack remotes (none configured; not copied and not configured as a side effect)"
	}
	return fmt.Sprintf("Rack remotes (configured at %s; not copied and not modified)", cfg.Endpoint)
}

func countFiles(root string) int {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return 0
	}
	n := 0
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}
