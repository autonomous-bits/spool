// Package ctxgit persists solution context as human-diffable files in a git
// remote. Git is the durable source of truth; the local SQLite projection is
// rebuilt from the checkout and is never committed or remoted.
package ctxgit

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	// BindRelPath is the explicit bind file in a code repository.
	BindRelPath = ".spool/context.toml"

	// DefaultProtectedBranch is used when the bind file omits protected_branch.
	DefaultProtectedBranch = "main"

	// DefaultLFSThreshold is the default Git LFS cutoff for non-text assets.
	DefaultLFSThreshold = 512 * 1024

	// LargeTextWarnBytes is the plain-git warning threshold for text/JSON/TOML.
	LargeTextWarnBytes = 1 << 20
)

// ErrUnbound reports that no explicit bind file was found. MCP write tools
// must fail closed; remotes are never inferred from directory names or leftovers.
var ErrUnbound = errors.New("workspace is not bound to a solution context remote")

// ErrInvalidBind reports a present bind file that is missing required fields.
var ErrInvalidBind = errors.New("context bind file is invalid")

// Bind is the explicit N→1 mapping from a code repo to the solution context remote.
type Bind struct {
	SolutionID      string `toml:"solution_id"`
	Remote          string `toml:"remote"`
	ProtectedBranch string `toml:"protected_branch"`
	RepositoryID    string `toml:"repository_id,omitempty"`
	LFSThresholdKiB *int   `toml:"lfs_threshold_kib,omitempty"`
}

// LFSThreshold returns the configured binary LFS cutoff in bytes.
func (b Bind) LFSThreshold() int64 {
	if b.LFSThresholdKiB != nil {
		if *b.LFSThresholdKiB <= 0 {
			return DefaultLFSThreshold
		}
		return int64(*b.LFSThresholdKiB) * 1024
	}
	return DefaultLFSThreshold
}

// UnboundError is the fail-closed envelope returned to bound context-management tools.
func UnboundError() error {
	return fmt.Errorf("%w: context-management commands require %s with solution_id, remote, and protected_branch (no auto-discovery)", ErrUnbound, BindRelPath)
}

// LoadBindFile parses an explicit bind file. It does not search ancestors.
func LoadBindFile(path string) (Bind, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Bind{}, fmt.Errorf("read bind file %s: %w", path, err)
	}
	var bind Bind
	if err := toml.Unmarshal(data, &bind); err != nil {
		return Bind{}, fmt.Errorf("%w: parse %s: %w", ErrInvalidBind, path, err)
	}
	bind.SolutionID = strings.TrimSpace(bind.SolutionID)
	bind.Remote = strings.TrimSpace(bind.Remote)
	bind.ProtectedBranch = strings.TrimSpace(bind.ProtectedBranch)
	bind.RepositoryID = strings.TrimSpace(bind.RepositoryID)
	if bind.ProtectedBranch == "" {
		bind.ProtectedBranch = DefaultProtectedBranch
	}
	if bind.SolutionID == "" {
		return Bind{}, fmt.Errorf("%w: %s missing solution_id", ErrInvalidBind, path)
	}
	if bind.Remote == "" {
		return Bind{}, fmt.Errorf("%w: %s missing remote", ErrInvalidBind, path)
	}
	return bind, nil
}

// FindBind resolves the explicit .spool/context.toml for the code repository
// containing start. It never infers a context remote from directory names,
// monorepo layout, go.work, .spl leftovers, git origin URLs, or Rack config.
//
// Lookup walks start and its ancestors looking only for BindRelPath. If a git
// work tree root (a directory containing .git) is reached without a bind file,
// search stops — nested checkouts do not inherit a parent bind. Finding no
// file is ErrUnbound; a present but invalid file is ErrInvalidBind.
func FindBind(start string) (codeRoot string, bindPath string, bind Bind, err error) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", "", Bind{}, err
	}
	for {
		candidate := filepath.Join(directory, BindRelPath)
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			bind, err = LoadBindFile(candidate)
			if err != nil {
				return "", candidate, Bind{}, err
			}
			if bind.RepositoryID == "" {
				bind.RepositoryID = filepath.Base(directory)
			}
			return directory, candidate, bind, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", "", Bind{}, statErr
		}
		if isGitWorkTreeRoot(directory) {
			return "", "", Bind{}, UnboundError()
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", "", Bind{}, UnboundError()
		}
		directory = parent
	}
}

func isGitWorkTreeRoot(directory string) bool {
	info, err := os.Stat(filepath.Join(directory, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

// WriteBindFile writes an explicit bind file, creating .spool if needed.
func WriteBindFile(codeRoot string, bind Bind) (string, error) {
	if bind.ProtectedBranch == "" {
		bind.ProtectedBranch = DefaultProtectedBranch
	}
	if bind.SolutionID == "" || bind.Remote == "" {
		return "", fmt.Errorf("%w: solution_id and remote are required", ErrInvalidBind)
	}
	path := filepath.Join(codeRoot, BindRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create bind directory: %w", err)
	}
	data, err := toml.Marshal(bind)
	if err != nil {
		return "", fmt.Errorf("encode bind file: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write bind file: %w", err)
	}
	return path, nil
}
