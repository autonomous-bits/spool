package ctxgit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
)

// StagedBatch is one in-process mutation batch waiting to become a single git commit.
type StagedBatch struct {
	Operations []repository.MutationOperation `json:"operations"`
	Assets     []StagedAsset                  `json:"-"`
	SchemaTOML []byte                         `json:"-"`
	Export     *ExportSummary                 `json:"-"`
	BaseCommit string                         `json:"baseCommit"`
}

// ExportSummary is honesty metadata attached to a migrate-once export commit.
type ExportSummary struct {
	Kept    []string `json:"kept"`
	Skipped []string `json:"skipped"`
	Lossy   bool     `json:"lossy"`
	Note    string   `json:"note"`
}

// StageResult summarizes a validated, not-yet-committed batch.
type StageResult struct {
	Branch     string `json:"branch"`
	BaseCommit string `json:"baseCommit"`
	Operations int    `json:"operations"`
}

// WriteResult is the outcome of one MCP mutation batch persisted to context git.
type WriteResult struct {
	Branch          string      `json:"branch"`
	Commit          string      `json:"commit"`
	ProtectedBranch string      `json:"protectedBranch"`
	Remote          string      `json:"remote"`
	PR              PullRequest `json:"pullRequest"`
	Overlaps        []Overlap   `json:"overlaps,omitempty"`
	Written         []string    `json:"written,omitempty"`
	Warnings        []string    `json:"warnings,omitempty"`
}

// Stage validates one mutation batch against the current checkout and holds it
// in process memory until Commit. It does not write git yet.
func (s *Session) Stage(operations []repository.MutationOperation) (StageResult, error) {
	if s == nil || !s.Bound() {
		return StageResult{}, UnboundError()
	}
	if len(operations) == 0 {
		return StageResult{}, repository.ErrInvalidMutationBatch
	}
	namespaced := namespaceOperations(s.Bind.RepositoryID, operations)
	normalized, err := normalizeOperations(namespaced)
	if err != nil {
		return StageResult{}, err
	}
	if err := validateBatch(s.graph, normalized); err != nil {
		return StageResult{}, err
	}
	if _, _, err := applyOperations(s.graph, normalized); err != nil {
		return StageResult{}, err
	}
	s.staged = &StagedBatch{Operations: normalized, BaseCommit: s.head}
	return StageResult{
		Branch:     s.Bind.ProtectedBranch,
		BaseCommit: s.head,
		Operations: len(normalized),
	}, nil
}

// StageAsset content-addresses a file and stages an Asset node in the same batch.
func (s *Session) StageAsset(filePath, id, title string) (repository.AssetAddResult, error) {
	if s == nil || !s.Bound() {
		return repository.AssetAddResult{}, UnboundError()
	}
	staged, err := ingestAsset(filePath, s.Bind.LFSThreshold())
	if err != nil {
		return repository.AssetAddResult{}, err
	}
	op := assetNodeOperation(staged, id, title, s.Bind.RepositoryID)
	ops := []repository.MutationOperation{op}
	if s.staged != nil {
		replaced := false
		ops = append([]repository.MutationOperation{}, s.staged.Operations...)
		for i, existing := range ops {
			if existing.Entity == "node" && existing.ID == op.ID {
				ops[i] = op
				replaced = true
				break
			}
		}
		if !replaced {
			ops = append(ops, op)
		}
	}
	normalized, err := normalizeOperations(ops)
	if err != nil {
		return repository.AssetAddResult{}, err
	}
	if err := validateBatch(s.graph, normalized); err != nil {
		return repository.AssetAddResult{}, err
	}
	assets := []StagedAsset{staged}
	if s.staged != nil {
		assets = append(append([]StagedAsset{}, s.staged.Assets...), staged)
	}
	s.staged = &StagedBatch{Operations: normalized, Assets: assets, BaseCommit: s.head}
	return repository.AssetAddResult{
		Node:     op.ID,
		AssetURI: staged.RelPath,
		Hash:     staged.Hash,
		Size:     staged.Size,
		MIMEType: staged.MIMEType,
		Branch:   s.Bind.ProtectedBranch,
		Staged:   true,
		Status:   "staged",
	}, nil
}

// Status reports the in-process staged batch for the bound context remote.
func (s *Session) Status() (StageResult, error) {
	if s == nil || !s.Bound() {
		return StageResult{}, UnboundError()
	}
	result := StageResult{Branch: s.Bind.ProtectedBranch, BaseCommit: s.head}
	if s.staged != nil {
		result.BaseCommit = s.staged.BaseCommit
		result.Operations = len(s.staged.Operations)
	}
	return result, nil
}

// Commit applies the staged batch as one git commit on a short-lived branch
// and opens a pull request to the protected branch. It never pushes the
// protected branch.
func (s *Session) Commit(ctx context.Context, author, message string) (WriteResult, error) {
	if s == nil || !s.Bound() {
		return WriteResult{}, UnboundError()
	}
	if s.staged == nil || len(s.staged.Operations) == 0 {
		return WriteResult{}, repository.ErrNoStagedMutations
	}
	if strings.TrimSpace(message) == "" {
		return WriteResult{}, fmt.Errorf("commit message is required")
	}
	if err := s.syncProtected(ctx); err != nil {
		return WriteResult{}, err
	}
	base, err := LoadGraph(s.CheckoutDir)
	if err != nil {
		return WriteResult{}, err
	}
	if len(s.staged.SchemaTOML) > 0 {
		base.SchemaTOML = append([]byte(nil), s.staged.SchemaTOML...)
	}
	s.graph = base
	if err := validateBatch(base, s.staged.Operations); err != nil {
		return WriteResult{}, err
	}
	after, overlaps, err := applyOperations(base, s.staged.Operations)
	if err != nil {
		return WriteResult{}, err
	}
	if len(s.staged.SchemaTOML) > 0 {
		after.SchemaTOML = append([]byte(nil), s.staged.SchemaTOML...)
	}
	return s.commitGraphDiff(ctx, base, after, overlaps, author, message, s.Bind.ProtectedBranch)
}

func (s *Session) finishWrite(ctx context.Context, after *Graph, branch, sha string, pr PullRequest, overlaps []Overlap, written, warnings []string) (WriteResult, error) {
	s.graph = after
	if rebuildErr := s.rebuildProjection(ctx, sha); rebuildErr != nil {
		warnings = append(warnings, "projection rebuild after write: "+rebuildErr.Error())
	}
	return WriteResult{
		Branch:          branch,
		Commit:          sha,
		ProtectedBranch: s.Bind.ProtectedBranch,
		Remote:          s.Bind.Remote,
		PR:              pr,
		Overlaps:        overlaps,
		Written:         written,
		Warnings:        warnings,
	}, nil
}

// CommitGraph writes after as one git commit on a short-lived branch from the
// current checkout HEAD and opens a pull request to prBase (protected branch
// when empty). It does not use in-process staging.
func (s *Session) CommitGraph(ctx context.Context, after *Graph, author, message, prBase string) (WriteResult, error) {
	if s == nil || !s.Bound() {
		return WriteResult{}, UnboundError()
	}
	if after == nil {
		return WriteResult{}, repository.ErrInvalidMutationBatch
	}
	if strings.TrimSpace(message) == "" {
		return WriteResult{}, fmt.Errorf("commit message is required")
	}
	if strings.TrimSpace(prBase) == "" {
		prBase = s.Bind.ProtectedBranch
	}
	base, err := LoadGraph(s.CheckoutDir)
	if err != nil {
		return WriteResult{}, err
	}
	return s.commitGraphDiff(ctx, base, after, nil, author, message, prBase)
}

func (s *Session) commitGraphDiff(ctx context.Context, base, after *Graph, overlaps []Overlap, author, message, prBase string) (WriteResult, error) {
	branch, err := shortLivedBranch()
	if err != nil {
		return WriteResult{}, err
	}
	if _, err := s.git.Run(ctx, s.CheckoutDir, "checkout", "-B", branch, "HEAD"); err != nil {
		return WriteResult{}, err
	}

	var warnings []string
	if s.staged != nil {
		for _, blob := range s.staged.Assets {
			if blob.Warn != "" {
				warnings = append(warnings, blob.Warn)
			}
			if err := persistAsset(ctx, s.git, s.CheckoutDir, blob); err != nil {
				return WriteResult{}, err
			}
		}
	}
	written, deleted, err := persistGraphDiff(s.CheckoutDir, base, after)
	if err != nil {
		return WriteResult{}, err
	}
	if err := persistSchema(s.CheckoutDir, after.SchemaTOML); err != nil {
		return WriteResult{}, err
	}
	for _, rel := range written {
		if shouldWarnLargeText(rel, fileSize(s.CheckoutDir, rel)) {
			warnings = append(warnings, fmt.Sprintf("%s exceeds ~1 MiB and stays in plain git", rel))
		}
	}
	if err := rejectForbiddenPaths(written); err != nil {
		return WriteResult{}, err
	}
	if err := rejectForbiddenPaths(deleted); err != nil {
		return WriteResult{}, err
	}

	if _, err := s.git.Run(ctx, s.CheckoutDir, "add", "-A", "schema.toml", "nodes", "edges", "assets", "README.md", ".gitignore", ".gitattributes"); err != nil {
		if _, addErr := s.git.Run(ctx, s.CheckoutDir, "add", "-A"); addErr != nil {
			return WriteResult{}, addErr
		}
	}
	if err := s.assertCachedPathsAllowed(ctx); err != nil {
		return WriteResult{}, err
	}
	status, err := s.git.Run(ctx, s.CheckoutDir, "status", "--porcelain")
	if err != nil {
		return WriteResult{}, err
	}
	if strings.TrimSpace(status) == "" {
		if _, checkoutErr := s.git.Run(ctx, s.CheckoutDir, "checkout", s.Bind.ProtectedBranch); checkoutErr != nil {
			warnings = append(warnings, "checkout protected branch after no-op: "+checkoutErr.Error())
		} else if _, delErr := s.git.Run(ctx, s.CheckoutDir, "branch", "-D", branch); delErr != nil {
			warnings = append(warnings, "delete unused short-lived branch: "+delErr.Error())
		}
		warnings = append(warnings, "no changes")
		s.staged = nil
		sha := s.head
		if sha == "" {
			if resolved, revErr := s.git.Run(ctx, s.CheckoutDir, "rev-parse", "HEAD"); revErr == nil {
				sha = resolved
			}
		}
		return s.finishWrite(ctx, after, s.Bind.ProtectedBranch, sha, PullRequest{}, overlaps, written, warnings)
	}
	commitArgs := []string{"-c", "commit.gpgsign=false"}
	if name, email := parseAuthor(author); name != "" {
		commitArgs = append(commitArgs, "-c", "user.name="+name, "-c", "user.email="+email)
	}
	commitArgs = append(commitArgs, "commit", "-m", message)
	if _, err := s.git.Run(ctx, s.CheckoutDir, commitArgs...); err != nil {
		return WriteResult{}, err
	}
	sha, err := s.git.Run(ctx, s.CheckoutDir, "rev-parse", "HEAD")
	if err != nil {
		return WriteResult{}, err
	}
	if _, err := s.git.Run(ctx, s.CheckoutDir, "push", "--set-upstream", "origin", branch); err != nil {
		return WriteResult{}, fmt.Errorf("push short-lived branch (stock git only): %w", err)
	}

	var export *ExportSummary
	if s.staged != nil {
		export = s.staged.Export
	}
	pr, err := s.prs.OpenPR(ctx, OpenPRRequest{
		Remote: s.Bind.Remote,
		Head:   branch,
		Base:   prBase,
		Title:  message,
		Body:   prBody(s.Bind, overlaps, written, deleted, warnings, export),
	})
	if err != nil {
		return WriteResult{}, err
	}
	s.staged = nil
	return s.finishWrite(ctx, after, branch, sha, pr, overlaps, written, warnings)
}

func shortLivedBranch() (string, error) {
	var nonce [4]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102t150405z")
	return fmt.Sprintf("spool/mcp/%s-%s", stamp, hex.EncodeToString(nonce[:])), nil
}

func parseAuthor(author string) (name, email string) {
	author = strings.TrimSpace(author)
	if author == "" {
		return "spool-mcp", "spool-mcp@localhost"
	}
	if i := strings.IndexByte(author, '<'); i >= 0 {
		name = strings.TrimSpace(author[:i])
		email = strings.TrimSuffix(strings.TrimSpace(author[i+1:]), ">")
		if name == "" {
			name = "spool-mcp"
		}
		if email == "" {
			email = "spool-mcp@localhost"
		}
		return name, email
	}
	return author, "spool-mcp@localhost"
}

func (s *Session) assertCachedPathsAllowed(ctx context.Context) error {
	listed, err := s.git.Run(ctx, s.CheckoutDir, "diff", "--cached", "--name-only")
	if err != nil {
		return err
	}
	var paths []string
	for _, line := range strings.Split(listed, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return rejectForbiddenPaths(paths)
}

func prBody(bind Bind, overlaps []Overlap, written, deleted, warnings []string, export *ExportSummary) string {
	var b strings.Builder
	if export != nil {
		b.WriteString("One-shot `.spl` → context git export (`export` / `migrate-once`). Not sync.\n\n")
		b.WriteString("Lossy is OK. Re-run is overwrite-at-own-risk. No dual-SoT window. Rack remotes are not configured as a side effect.\n\n")
		fmt.Fprintf(&b, "- Solution: `%s`\n- Protected branch: `%s`\n\n", bind.SolutionID, bind.ProtectedBranch)
		if len(export.Kept) > 0 {
			b.WriteString("## Kept\n\n")
			for _, item := range export.Kept {
				fmt.Fprintf(&b, "- %s\n", item)
			}
			b.WriteString("\n")
		}
		if len(export.Skipped) > 0 {
			b.WriteString("## Skipped\n\n")
			for _, item := range export.Skipped {
				fmt.Fprintf(&b, "- %s\n", item)
			}
			b.WriteString("\n")
		}
	} else {
		b.WriteString("Spool MCP context write (one mutation batch → one git commit).\n\n")
		fmt.Fprintf(&b, "- Solution: `%s`\n- Protected branch: `%s`\n\n", bind.SolutionID, bind.ProtectedBranch)
	}
	if len(overlaps) > 0 {
		b.WriteString("## Overlaps (human review required — no silent overwrite)\n\n")
		for _, overlap := range overlaps {
			fmt.Fprintf(&b, "- `%s` %s (`%s`)\n", overlap.Entity, overlap.ID, overlap.Path)
		}
		b.WriteString("\n")
	}
	if len(written) > 0 {
		b.WriteString("## Written paths\n\n")
		for _, path := range written {
			fmt.Fprintf(&b, "- `%s`\n", path)
		}
		b.WriteString("\n")
	}
	if len(deleted) > 0 {
		b.WriteString("## Deleted paths\n\n")
		for _, path := range deleted {
			fmt.Fprintf(&b, "- `%s`\n", path)
		}
		b.WriteString("\n")
	}
	if len(warnings) > 0 {
		b.WriteString("## Warnings\n\n")
		for _, warning := range warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
	}
	return b.String()
}

func fileSize(root, rel string) int64 {
	info, err := osStatJoin(root, rel)
	if err != nil {
		return 0
	}
	return info.Size()
}
