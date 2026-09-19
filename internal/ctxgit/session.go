package ctxgit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository/asset"
)

// Session is the process-local bound context: a stock git checkout plus a
// disposable projection rebuilt from that checkout.
type Session struct {
	Bind           Bind
	CodeRoot       string
	BindPath       string
	CacheDir       string
	CheckoutDir    string
	head           string
	graph          *Graph
	staged         *StagedBatch
	git            GitRunner
	prs            PROpener
	projectionPath string
}

// Options configure session startup. CacheDir and PROpener are test seams.
type Options struct {
	WorkspaceDir string
	CacheDir     string
	Git          GitRunner
	PROpener     PROpener
}

// Start resolves the explicit bind, updates the local checkout from the
// configured remote with stock git, and rebuilds the local projection.
func Start(ctx context.Context, opts Options) (*Session, error) {
	workspace := opts.WorkspaceDir
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		workspace = wd
	}
	codeRoot, bindPath, bind, err := FindBind(workspace)
	if err != nil {
		return nil, err
	}
	s := &Session{
		Bind:     bind,
		CodeRoot: codeRoot,
		BindPath: bindPath,
		git:      opts.Git,
		prs:      opts.PROpener,
	}
	if s.git.Bin == "" && len(s.git.Env) == 0 {
		s.git = defaultGit()
	}
	if s.prs == nil {
		s.prs = defaultPROpener()
	}
	s.CacheDir = opts.CacheDir
	if s.CacheDir == "" {
		s.CacheDir = defaultCacheDir(bind)
	}
	s.CheckoutDir = filepath.Join(s.CacheDir, "checkout")
	if err := s.ensureCheckout(ctx); err != nil {
		return nil, err
	}
	if err := s.syncProtected(ctx); err != nil {
		return nil, err
	}
	graph, err := LoadGraph(s.CheckoutDir)
	if err != nil {
		return nil, err
	}
	s.graph = graph
	if err := s.rebuildProjection(ctx, s.head); err != nil {
		return nil, fmt.Errorf("rebuild local projection from checkout: %w", err)
	}
	return s, nil
}

// Bound reports whether this session has an explicit bind.
func (s *Session) Bound() bool {
	return s != nil && s.Bind.Remote != "" && s.Bind.SolutionID != ""
}

func defaultCacheDir(bind Bind) string {
	if override := strings.TrimSpace(os.Getenv("SPOOL_CONTEXT_CACHE")); override != "" {
		return filepath.Join(override, sanitizeCacheSegment(bind.SolutionID))
	}
	root, err := os.UserCacheDir()
	if err != nil {
		sum := sha256.Sum256([]byte(bind.Remote))
		return filepath.Join(os.TempDir(), "spool-context", hex.EncodeToString(sum[:8]))
	}
	return filepath.Join(root, "spool", "context", sanitizeCacheSegment(bind.SolutionID))
}

func sanitizeCacheSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unnamed"
	}
	var b strings.Builder
	for _, r := range value {
		if r == '/' || r == '\\' || r == ':' || r < 32 {
			b.WriteByte('-')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *Session) ensureCheckout(ctx context.Context) error {
	if err := os.MkdirAll(s.CacheDir, 0o755); err != nil {
		return err
	}
	gitDir := filepath.Join(s.CheckoutDir, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		if _, err := s.git.Run(ctx, s.CheckoutDir, "remote", "set-url", "origin", s.Bind.Remote); err != nil {
			return err
		}
		return nil
	}
	if err := os.RemoveAll(s.CheckoutDir); err != nil {
		return err
	}
	if _, err := s.git.Run(ctx, s.CacheDir, "clone", "--branch", s.Bind.ProtectedBranch, s.Bind.Remote, s.CheckoutDir); err == nil {
		return nil
	}
	// Empty remotes cannot be cloned; initialize a local checkout and fetch if possible.
	if err := os.MkdirAll(s.CheckoutDir, 0o755); err != nil {
		return err
	}
	if _, err := s.git.Run(ctx, s.CheckoutDir, "init", "-b", s.Bind.ProtectedBranch); err != nil {
		return err
	}
	if _, err := s.git.Run(ctx, s.CheckoutDir, "remote", "add", "origin", s.Bind.Remote); err != nil {
		return err
	}
	_, _ = s.git.Run(ctx, s.CheckoutDir, "fetch", "origin", s.Bind.ProtectedBranch)
	if s.git.HasRev(ctx, s.CheckoutDir, "origin/"+s.Bind.ProtectedBranch) {
		if _, err := s.git.Run(ctx, s.CheckoutDir, "checkout", "-B", s.Bind.ProtectedBranch, "origin/"+s.Bind.ProtectedBranch); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) syncProtected(ctx context.Context) error {
	_, _ = s.git.Run(ctx, s.CheckoutDir, "fetch", "origin", s.Bind.ProtectedBranch)
	switch {
	case s.git.HasRev(ctx, s.CheckoutDir, "origin/"+s.Bind.ProtectedBranch):
		if _, err := s.git.Run(ctx, s.CheckoutDir, "checkout", "--force", "-B", s.Bind.ProtectedBranch, "origin/"+s.Bind.ProtectedBranch); err != nil {
			return err
		}
	case s.git.HasRev(ctx, s.CheckoutDir, "refs/heads/"+s.Bind.ProtectedBranch):
		if _, err := s.git.Run(ctx, s.CheckoutDir, "checkout", "--force", s.Bind.ProtectedBranch); err != nil {
			return err
		}
	default:
		if err := EnsureLayout(s.CheckoutDir); err != nil {
			return err
		}
		s.head = ""
		return nil
	}
	sha, err := s.git.Run(ctx, s.CheckoutDir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	s.head = sha
	return nil
}

// ReadAsset streams a blob from the current checkout (never from a remoted projection).
func (s *Session) ReadAsset(_ context.Context, locator string) (io.ReadCloser, int64, asset.Metadata, error) {
	if s == nil || !s.Bound() {
		return nil, 0, asset.Metadata{}, UnboundError()
	}
	if node, ok := s.ResolveNode(locator); ok {
		if path, ok := node.Properties["assetPath"]; ok && path.String != "" {
			locator = path.String
		} else if hash, ok := node.Properties["hash"]; ok && hash.String != "" {
			locator = hash.String
		}
	}
	return readCheckoutAsset(s.CheckoutDir, locator)
}
