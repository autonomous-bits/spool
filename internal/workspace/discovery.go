package workspace

import (
	"fmt"
	"path/filepath"
)

// WorkspaceMatch identifies the registry entry that owns a working directory.
type WorkspaceMatch struct {
	Name      Name
	Workspace Workspace
}

// FindWorkspace loads the registry below root and returns the workspace whose
// attached repository path is the longest component-boundary prefix of
// workingDirectory. It canonicalizes workingDirectory before matching.
//
// It returns ErrWorkspaceNotFound when no attached repository contains
// workingDirectory. Registry and filesystem errors are returned unchanged or
// wrapped and are distinct from ErrWorkspaceNotFound.
func FindWorkspace(root, workingDirectory string) (WorkspaceMatch, error) {
	canonicalWorkingDirectory, err := CanonicalPath(workingDirectory)
	if err != nil {
		return WorkspaceMatch{}, fmt.Errorf("canonicalize working directory: %w", err)
	}

	registry, err := LoadRegistry(root)
	if err != nil {
		return WorkspaceMatch{}, err
	}

	var match WorkspaceMatch
	matchedPathLength := -1
	for name, workspace := range registry.Workspaces {
		for _, attachedPath := range workspace.Paths {
			if !pathContains(attachedPath, canonicalWorkingDirectory) {
				continue
			}
			if len(attachedPath) > matchedPathLength {
				match = WorkspaceMatch{Name: name, Workspace: workspace}
				matchedPathLength = len(attachedPath)
			}
		}
	}
	if matchedPathLength == -1 {
		return WorkspaceMatch{}, fmt.Errorf("%w: %q", ErrWorkspaceNotFound, canonicalWorkingDirectory)
	}
	return match, nil
}

func pathContains(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !filepath.IsAbs(relative) && !hasParentPathPrefix(relative)
}

func hasParentPathPrefix(path string) bool {
	return len(path) > 3 && path[:3] == ".."+string(filepath.Separator)
}
