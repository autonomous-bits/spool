package ctxgit

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// ErrForbiddenContextPath reports that a pack, Rack remote, reflog, merge
// lease, projection, or other dropped .spl artifact would be committed to the
// context tree. Export fails closed rather than copying those paths.
var ErrForbiddenContextPath = errors.New("forbidden path would land in the context tree")

func forbiddenContextPath(rel string) bool {
	rel = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/")))
	rel = strings.TrimPrefix(rel, "./")
	if rel == "" || rel == "." {
		return false
	}
	base := path.Base(rel)
	for _, part := range strings.Split(rel, "/") {
		switch part {
		case ".spl", "objects", "pack", "logs", "merge", "reflog", "reflogs":
			return true
		}
	}
	switch base {
	case "graph.db", "projection.db", "repository.json":
		return true
	}
	if strings.HasSuffix(base, ".pack") ||
		strings.HasSuffix(base, ".db") ||
		strings.HasSuffix(base, ".db-wal") ||
		strings.HasSuffix(base, ".db-shm") {
		return true
	}
	return false
}

func rejectForbiddenPaths(paths []string) error {
	var hit []string
	seen := map[string]struct{}{}
	for _, rel := range paths {
		if !forbiddenContextPath(rel) {
			continue
		}
		if _, dup := seen[rel]; dup {
			continue
		}
		seen[rel] = struct{}{}
		hit = append(hit, rel)
	}
	if len(hit) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s (packs, Rack remotes, reflogs, merge leases, and projections must not land in the context tree)", ErrForbiddenContextPath, strings.Join(hit, ", "))
}
