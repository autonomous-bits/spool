// Package workspace defines workspace values and detached storage locations.
package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxNameLength     = 63
	workspaceIDPrefix = "ws_"
	workspaceIDBytes  = 4
	workspaceIDLength = len(workspaceIDPrefix) + workspaceIDBytes*2
)

var (
	// ErrInvalidName reports a workspace name that cannot safely identify a
	// registry entry.
	ErrInvalidName = errors.New("workspace name is invalid")
	// ErrInvalidID reports a workspace identity that does not match the
	// generated-ID format.
	ErrInvalidID = errors.New("workspace identity is invalid")
	// ErrInvalidWorkspace reports incomplete or unsafe workspace registry data.
	ErrInvalidWorkspace = errors.New("workspace is invalid")
)

// Name identifies a workspace by its stable, user-selected slug.
type Name string

// ParseName validates value and returns its workspace-name representation.
func ParseName(value string) (Name, error) {
	name := Name(value)
	if err := name.Validate(); err != nil {
		return "", err
	}
	return name, nil
}

// Validate reports whether name is a lowercase ASCII slug suitable for use as
// a registry key.
func (name Name) Validate() error {
	value := string(name)
	if len(value) == 0 || len(value) > maxNameLength || !utf8.ValidString(value) {
		return ErrInvalidName
	}
	if value[0] == '-' || value[len(value)-1] == '-' {
		return fmt.Errorf("%w: must begin and end with a letter or digit", ErrInvalidName)
	}
	if strings.Contains(value, "--") {
		return fmt.Errorf("%w: must not contain consecutive hyphens", ErrInvalidName)
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' {
			continue
		}
		return fmt.Errorf("%w: %q contains unsupported characters", ErrInvalidName, value)
	}
	return nil
}

// ID identifies a workspace independently of its user-facing name.
type ID string

// NewID creates a random durable workspace identity in the form ws_ followed by
// eight lowercase hexadecimal characters.
func NewID() (ID, error) {
	var random [workspaceIDBytes]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		return "", fmt.Errorf("generate workspace identity: %w", err)
	}
	return ID(workspaceIDPrefix + hex.EncodeToString(random[:])), nil
}

// ParseID validates value and returns its workspace identity representation.
func ParseID(value string) (ID, error) {
	id := ID(value)
	if err := id.Validate(); err != nil {
		return "", err
	}
	return id, nil
}

// Validate reports whether id matches the generated workspace-ID format.
func (id ID) Validate() error {
	value := string(id)
	if len(value) != workspaceIDLength || !strings.HasPrefix(value, workspaceIDPrefix) {
		return ErrInvalidID
	}
	for _, character := range value[len(workspaceIDPrefix):] {
		if character < '0' || (character > '9' && character < 'a') || character > 'f' {
			return ErrInvalidID
		}
	}
	return nil
}

// Workspace is a registered workspace's domain representation.
type Workspace struct {
	ID        ID        `toml:"id" json:"id"`
	Name      string    `toml:"name" json:"name"`
	StateDir  string    `toml:"state_dir" json:"stateDir"`
	CreatedAt time.Time `toml:"created_at" json:"createdAt"`
	Paths     []string  `toml:"paths" json:"paths"`
}

// Validate reports whether workspace has all documented durable fields. Attached
// paths are normalized and collision-checked by the registry store.
func (workspace Workspace) Validate() error {
	if err := workspace.ID.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidWorkspace, err)
	}
	if strings.TrimSpace(workspace.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidWorkspace)
	}
	if !filepath.IsAbs(workspace.StateDir) {
		return fmt.Errorf("%w: state directory must be absolute", ErrInvalidWorkspace)
	}
	if workspace.CreatedAt.IsZero() {
		return fmt.Errorf("%w: creation timestamp is required", ErrInvalidWorkspace)
	}
	return nil
}

// CreateRequest identifies the workspace to create. It contains no storage or
// persistence policy so callers can resolve a location before initialization.
type CreateRequest struct {
	Name Name `json:"name"`
	ID   ID   `json:"id"`
}

// NewCreateRequest creates a validated workspace-creation request with a new
// identity, keeping identity generation separate from the workspace name.
func NewCreateRequest(name Name) (CreateRequest, error) {
	if err := name.Validate(); err != nil {
		return CreateRequest{}, err
	}
	id, err := NewID()
	if err != nil {
		return CreateRequest{}, err
	}
	return CreateRequest{Name: name, ID: id}, nil
}

// Validate reports whether request contains a valid workspace name and ID.
func (request CreateRequest) Validate() error {
	if err := request.Name.Validate(); err != nil {
		return err
	}
	return request.ID.Validate()
}
