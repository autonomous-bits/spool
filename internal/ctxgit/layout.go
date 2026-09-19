package ctxgit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/autonomous-bits/spool/graphcontract"
	"github.com/autonomous-bits/spool/internal/repository"
)

const (
	schemaFileName = "schema.toml"
	nodesDirName   = "nodes"
	edgesDirName   = "edges"
	assetsDirName  = "assets"
	readmeFileName = "README.md"
)

const defaultSchemaTOML = "version = 1\npermissive = true\n"

const defaultREADME = `# Solution context

This repository is the durable, human-diffable source of truth for shared
solution context. Agents write through Spool MCP (short-lived branch + PR).
Local query projections are rebuilt from this checkout and must never be committed.
`

// Graph is the in-memory view of a context checkout. It is not a second store.
type Graph struct {
	SchemaTOML []byte
	Nodes      map[string]repository.Node
	Edges      map[string]repository.Edge
	// NodeFiles and EdgeFiles map entity IDs to relative checkout paths.
	NodeFiles map[string]string
	EdgeFiles map[string]string
}

// NewGraph returns an empty graph with the built-in permissive schema.
func NewGraph() *Graph {
	return &Graph{
		SchemaTOML: []byte(defaultSchemaTOML),
		Nodes:      map[string]repository.Node{},
		Edges:      map[string]repository.Edge{},
		NodeFiles:  map[string]string{},
		EdgeFiles:  map[string]string{},
	}
}

// EnsureLayout creates the agreed context-repo directories and default files
// when they are missing. Existing files are left untouched.
func EnsureLayout(root string) error {
	for _, dir := range []string{nodesDirName, edgesDirName, assetsDirName} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	schemaPath := filepath.Join(root, schemaFileName)
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		if err := os.WriteFile(schemaPath, []byte(defaultSchemaTOML), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", schemaFileName, err)
		}
	} else if err != nil {
		return err
	}
	readmePath := filepath.Join(root, readmeFileName)
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		if err := os.WriteFile(readmePath, []byte(defaultREADME), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", readmeFileName, err)
		}
	} else if err != nil {
		return err
	}
	ignorePath := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(ignorePath); os.IsNotExist(err) {
		ignore := strings.Join([]string{
			"# Local projections are never source of truth",
			"*.db",
			"*.db-wal",
			"*.db-shm",
			".spl/",
			"",
		}, "\n")
		if err := os.WriteFile(ignorePath, []byte(ignore), 0o644); err != nil {
			return fmt.Errorf("write .gitignore: %w", err)
		}
	} else if err != nil {
		return err
	}
	return nil
}

// LoadGraph reads human-diffable JSON/TOML from a context checkout.
func LoadGraph(root string) (*Graph, error) {
	graph := NewGraph()
	schemaPath := filepath.Join(root, schemaFileName)
	if data, err := os.ReadFile(schemaPath); err == nil {
		graph.SchemaTOML = data
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", schemaFileName, err)
	}
	if err := loadEntities(root, nodesDirName, func(rel string, data []byte) error {
		var node repository.Node
		if err := json.Unmarshal(data, &node); err != nil {
			return fmt.Errorf("decode %s: %w", rel, err)
		}
		if node.ID == "" {
			return fmt.Errorf("node file %s is missing id", rel)
		}
		normalized, err := node.Normalize()
		if err != nil {
			return fmt.Errorf("normalize node %s: %w", node.ID, err)
		}
		graph.Nodes[normalized.ID] = normalized
		graph.NodeFiles[normalized.ID] = rel
		return nil
	}); err != nil {
		return nil, err
	}
	if err := loadEntities(root, edgesDirName, func(rel string, data []byte) error {
		var edge repository.Edge
		if err := json.Unmarshal(data, &edge); err != nil {
			return fmt.Errorf("decode %s: %w", rel, err)
		}
		if edge.ID == "" {
			return fmt.Errorf("edge file %s is missing id", rel)
		}
		normalized, err := edge.Normalize()
		if err != nil {
			return fmt.Errorf("normalize edge %s: %w", edge.ID, err)
		}
		graph.Edges[normalized.ID] = normalized
		graph.EdgeFiles[normalized.ID] = rel
		return nil
	}); err != nil {
		return nil, err
	}
	return graph, nil
}

func loadEntities(root, dirName string, fn func(rel string, data []byte) error) error {
	dir := filepath.Join(root, dirName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return fn(filepath.ToSlash(rel), data)
	})
}

func nodeRelPath(id string) (string, error) {
	name, err := FileName(id)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(nodesDirName, name)), nil
}

func edgeRelPath(id string) (string, error) {
	name, err := FileName(id)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(edgesDirName, name)), nil
}

func encodePrettyJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeSchemaFile(root string, data []byte) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, schemaFileName), data, 0o644)
}

func writeJSONFile(root, rel string, v any) error {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	data, err := encodePrettyJSON(v)
	if err != nil {
		return err
	}
	return os.WriteFile(abs, data, 0o644)
}

func schemaSnapshot(graph *Graph) (repository.SchemaSnapshot, error) {
	if len(graph.SchemaTOML) == 0 {
		return graphcontract.BuiltinSchemaSnapshot(), nil
	}
	schema, err := repository.DecodeSchemaTOML(graph.SchemaTOML)
	if err != nil {
		return repository.SchemaSnapshot{}, err
	}
	return schema, nil
}
