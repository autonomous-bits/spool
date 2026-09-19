package ctxgit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/autonomous-bits/spool/internal/repository"
)

// InitRequest bootstraps a solution context remote and writes the explicit bind.
type InitRequest struct {
	WorkspaceDir    string
	Remote          string
	SolutionID      string
	ProtectedBranch string
	RepositoryID    string
	Author          string
}

// InitResult is the JSON payload for `spl context init`.
type InitResult struct {
	BindPath        string `json:"bindPath"`
	SolutionID      string `json:"solutionId"`
	Remote          string `json:"remote"`
	ProtectedBranch string `json:"protectedBranch"`
	RepositoryID    string `json:"repositoryId"`
	Checkout        string `json:"checkout"`
	SeededNode      string `json:"seededNode,omitempty"`
	Pushed          bool   `json:"pushed"`
}

// Init writes the explicit bind, creates the agreed layout on an empty remote,
// and seeds a CodeRepository node for this code repo. This is onboarding, not
// an MCP mutation — MCP writes still use a short-lived branch + PR.
func Init(ctx context.Context, req InitRequest, git GitRunner) (InitResult, error) {
	if strings.TrimSpace(req.Remote) == "" {
		return InitResult{}, fmt.Errorf("remote is required")
	}
	workspace := req.WorkspaceDir
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return InitResult{}, err
		}
		workspace = wd
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return InitResult{}, err
	}
	if git.Bin == "" && len(git.Env) == 0 {
		git = defaultGit()
	}
	bind := Bind{
		SolutionID:      req.SolutionID,
		Remote:          req.Remote,
		ProtectedBranch: req.ProtectedBranch,
		RepositoryID:    req.RepositoryID,
	}
	if bind.ProtectedBranch == "" {
		bind.ProtectedBranch = DefaultProtectedBranch
	}
	if bind.SolutionID == "" {
		bind.SolutionID = filepath.Base(workspace)
	}
	if bind.RepositoryID == "" {
		bind.RepositoryID = filepath.Base(workspace)
	}
	bindPath, err := WriteBindFile(workspace, bind)
	if err != nil {
		return InitResult{}, err
	}

	opts := Options{WorkspaceDir: workspace, Git: git, PROpener: &RecordingPROpener{}}
	if cache := strings.TrimSpace(os.Getenv("SPOOL_CONTEXT_CACHE")); cache != "" {
		opts.CacheDir = filepath.Join(cache, sanitizeCacheSegment(bind.SolutionID))
	}
	session, err := Start(ctx, opts)
	if err != nil {
		return InitResult{}, err
	}
	if err := EnsureLayout(session.CheckoutDir); err != nil {
		return InitResult{}, err
	}

	seeded, err := seedCodeRepository(session.CheckoutDir, bind)
	if err != nil {
		return InitResult{}, err
	}

	pushed := false
	if _, err := git.Run(ctx, session.CheckoutDir, "add", "-A"); err != nil {
		return InitResult{}, err
	}
	if dirty, err := checkoutDirty(ctx, git, session.CheckoutDir); err != nil {
		return InitResult{}, err
	} else if dirty {
		authorName, authorEmail := parseAuthor(req.Author)
		if _, err := git.Run(ctx, session.CheckoutDir,
			"-c", "commit.gpgsign=false",
			"-c", "user.name="+authorName,
			"-c", "user.email="+authorEmail,
			"commit", "-m", "Initialize solution context layout"); err != nil {
			return InitResult{}, err
		}
	}
	if _, err := git.Run(ctx, session.CheckoutDir, "push", "-u", "origin", "HEAD:"+bind.ProtectedBranch); err != nil {
		return InitResult{}, fmt.Errorf("push context layout with stock git: %w", err)
	}
	pushed = true
	sha, _ := git.Run(ctx, session.CheckoutDir, "rev-parse", "HEAD")
	session.head = sha
	graph, err := LoadGraph(session.CheckoutDir)
	if err != nil {
		return InitResult{}, err
	}
	session.graph = graph
	if err := session.rebuildProjection(ctx, sha); err != nil {
		return InitResult{}, err
	}

	return InitResult{
		BindPath:        bindPath,
		SolutionID:      bind.SolutionID,
		Remote:          bind.Remote,
		ProtectedBranch: bind.ProtectedBranch,
		RepositoryID:    bind.RepositoryID,
		Checkout:        session.CheckoutDir,
		SeededNode:      seeded,
		Pushed:          pushed,
	}, nil
}

func seedCodeRepository(root string, bind Bind) (string, error) {
	graph, err := LoadGraph(root)
	if err != nil {
		return "", err
	}
	id := NamespaceID(bind.RepositoryID, "coderepo")
	if _, exists := graph.Nodes[id]; exists {
		return "", nil
	}
	node := repository.Node{
		ID:     id,
		Title:  bind.RepositoryID,
		Labels: []string{"CodeRepository"},
		Properties: map[string]repository.PropertyValue{
			"repositoryId": repository.StringPropertyValue(bind.RepositoryID),
			"solutionId":   repository.StringPropertyValue(bind.SolutionID),
		},
	}
	normalized, err := node.Normalize()
	if err != nil {
		return "", err
	}
	rel, err := nodeRelPath(id)
	if err != nil {
		return "", err
	}
	if err := writeJSONFile(root, rel, normalized); err != nil {
		return "", err
	}
	return id, nil
}

func checkoutDirty(ctx context.Context, git GitRunner, dir string) (bool, error) {
	status, err := git.Run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(status) != "", nil
}
