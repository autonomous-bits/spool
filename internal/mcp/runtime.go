package mcp

import (
	"context"
	"os"
	"sync"

	"github.com/autonomous-bits/spool/internal/ctxgit"
)

// ServerOptions configure MCP process startup, including the context-git session.
type ServerOptions struct {
	StateDir     func() (string, error)
	WorkspaceDir func() (string, error)
	CacheDir     string
	PROpener     ctxgit.PROpener
	Git          ctxgit.GitRunner
}

type runtime struct {
	stateDir     func() (string, error)
	workspaceDir func() (string, error)
	cacheDir     string
	prOpener     ctxgit.PROpener
	git          ctxgit.GitRunner

	mu      sync.Mutex
	session *ctxgit.Session
}

func newRuntime(opts ServerOptions) *runtime {
	if opts.StateDir == nil {
		opts.StateDir = func() (string, error) { return "", os.ErrNotExist }
	}
	if opts.WorkspaceDir == nil {
		opts.WorkspaceDir = os.Getwd
	}
	return &runtime{
		stateDir:     opts.StateDir,
		workspaceDir: opts.WorkspaceDir,
		cacheDir:     opts.CacheDir,
		prOpener:     opts.PROpener,
		git:          opts.Git,
	}
}

func (rt *runtime) start(ctx context.Context) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	_, _ = rt.ensureSessionLocked(ctx)
}

func (rt *runtime) ensureSessionLocked(ctx context.Context) (*ctxgit.Session, error) {
	if rt.session != nil {
		return rt.session, nil
	}
	workspace, err := rt.workspaceDir()
	if err != nil {
		return nil, err
	}
	session, err := ctxgit.Start(ctx, ctxgit.Options{
		WorkspaceDir: workspace,
		CacheDir:     rt.cacheDir,
		Git:          rt.git,
		PROpener:     rt.prOpener,
	})
	if err != nil {
		return nil, err
	}
	rt.session = session
	return session, nil
}

func (rt *runtime) requireSession(ctx context.Context) (*ctxgit.Session, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	session, err := rt.ensureSessionLocked(ctx)
	if err != nil {
		return nil, err
	}
	if session == nil || !session.Bound() {
		return nil, ctxgit.UnboundError()
	}
	return session, nil
}

func (rt *runtime) boundSession() *ctxgit.Session {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.session != nil && rt.session.Bound() {
		return rt.session
	}
	return nil
}
