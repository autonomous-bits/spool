// Package workspace defines workspace values and detached storage locations.
package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	maxNameLength     = 63
	workspaceIDPrefix = "ws_"
	workspaceIDBytes  = 4
	workspaceIDLength = len(workspaceIDPrefix) + 8
)

var (
	ErrInvalidName = errors.New("workspace name is invalid")
	// ErrInvalidID reports a workspace identity that does not match the
	// manifest format.
	ErrInvalidID = errors.New("workspace identity is invalid")
)

// Name is the user-selected name used only for explicit workspace provisioning.
type Name string

// ParseName validates the explicit provisioning name.
func ParseName(value string) (Name, error) {
	name := Name(value)
	if err := name.Validate(); err != nil {
		return "", err
	}
	return name, nil
}

func (name Name) Validate() error {
	value := string(name)
	if len(value) == 0 || len(value) > maxNameLength || !utf8.ValidString(value) {
		return ErrInvalidName
	}
	if value[0] == '-' || value[len(value)-1] == '-' || strings.Contains(value, "--") {
		return ErrInvalidName
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-' {
			continue
		}
		return ErrInvalidName
	}
	return nil
}

// ID identifies a workspace independently of checkout paths and names.
type ID string

// NewID creates a random durable workspace identity.
func NewID() (ID, error) {
	var random [workspaceIDBytes]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		return "", fmt.Errorf("generate workspace identity: %w", err)
	}
	return ID(workspaceIDPrefix + hex.EncodeToString(random[:])), nil
}

// Validate reports whether id matches the workspace-ID format.
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
