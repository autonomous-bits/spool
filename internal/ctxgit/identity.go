package ctxgit

import (
	"fmt"
	"strings"
	"unicode"
)

const namespaceSeparator = "/"

// NamespaceID prefixes id with repositoryID when id is not already namespaced.
// Already-namespaced IDs (containing "/") are left unchanged so cross-repo
// references stay first-class and files remain flat under nodes/ and edges/.
func NamespaceID(repositoryID, id string) string {
	id = strings.TrimSpace(id)
	repositoryID = strings.TrimSpace(repositoryID)
	if id == "" || repositoryID == "" || strings.Contains(id, namespaceSeparator) {
		return id
	}
	return repositoryID + namespaceSeparator + id
}

// FileName maps a graph ID onto a single filesystem-safe path segment so files
// stay flat under nodes/ and edges/. The JSON payload remains the ID authority.
func FileName(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("entity id is empty")
	}
	if strings.Contains(id, "..") {
		return "", fmt.Errorf("entity id %q must not contain '..'", id)
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r == '/':
			b.WriteString("--")
		case r < 127 && isFilenameSafe(r):
			b.WriteRune(r)
		default:
			if r <= 0xFF {
				_, _ = fmt.Fprintf(&b, "%%%02X", r)
			} else {
				_, _ = fmt.Fprintf(&b, "%%%X", r)
			}
		}
	}
	name := b.String()
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("entity id %q is not filesystem-safe", id)
	}
	return name + ".json", nil
}

func isFilenameSafe(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
}
