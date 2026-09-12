package mcp

import (
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

// withRepo opens the repository at stateDir, runs fn, and guarantees repo.Close() is called.
func withRepo[T any](stateDirProvider func() (string, error), fn func(*repository.Repository) (T, error)) (T, error) {
	stateDir, err := stateDirProvider()
	if err != nil {
		var zero T
		return zero, fmt.Errorf("locate state directory: %w", err)
	}
	repo, err := repository.OpenRepository(stateDir)
	if err != nil {
		var zero T
		return zero, err
	}
	defer repo.Close()
	return fn(repo)
}

// withTool opens the repository at stateDir, instantiates ResolveTool, and closes repo when done.
func withTool[T any](stateDirProvider func() (string, error), fn func(*resolve.ResolveTool) (T, error)) (T, error) {
	return withRepo(stateDirProvider, func(repo *repository.Repository) (T, error) {
		tool := resolve.NewResolveTool(repo)
		return fn(tool)
	})
}

// NewSpoolTools returns the suite of Spool MCP tools bound to the stateDirProvider.
// Each tool is defined in its own individual source file within this package.
func NewSpoolTools(stateDirProvider func() (string, error)) []Tool {
	return []Tool{
		// Staging & Working Changes
		toolStatus(stateDirProvider),
		toolAdd(stateDirProvider),
		toolCommit(stateDirProvider),

		// Graph Retrieval & Queries
		toolResolve(stateDirProvider),
		toolSearch(stateDirProvider),
		toolSearchExpand(stateDirProvider),
		toolFilter(stateDirProvider),
		toolContext(stateDirProvider),
		toolDiff(stateDirProvider),
		toolHistory(stateDirProvider),
		toolBranchesContaining(stateDirProvider),
		toolGraph(stateDirProvider),

		// Branch Management
		toolBranchList(stateDirProvider),
		toolBranchCreate(stateDirProvider),
		toolBranchDelete(stateDirProvider),
		toolSwitch(stateDirProvider),

		// Merge Operations
		toolMergePreview(stateDirProvider),
		toolMergeApply(stateDirProvider),
		toolMergeConflicts(stateDirProvider),
		toolMergeResolve(stateDirProvider),
		toolMergeAbort(stateDirProvider),
		toolMergeFinalize(stateDirProvider),

		// Advanced VCS
		toolCherryPick(stateDirProvider),

		// Schema & Validation
		toolSchemaMigrate(stateDirProvider),
		toolValidate(stateDirProvider),

		// Maintenance & Storage
		toolInit(stateDirProvider),
		toolPrune(stateDirProvider),
		toolFsck(stateDirProvider),
		toolGC(stateDirProvider),
		toolMigrate(stateDirProvider),

		// Reference Assets
		toolAssetAdd(stateDirProvider),
		toolAssetRead(stateDirProvider),

		// Workspaces
		toolWorkspaceInit(),
		toolWorkspaceAttach(),

		// Remotes & Networking
		toolRemoteSet(stateDirProvider),
		toolRemoteShow(stateDirProvider),
		toolRemoteRemove(stateDirProvider),
		toolRemoteBranch(stateDirProvider),
		toolPush(stateDirProvider),
		toolPull(stateDirProvider),
		toolClone(),

		// Metadata
		toolVersion(),
	}
}
