// Package branch defines local branch lifecycle operations.
package branch

import (
	"context"
	"errors"
)

var (
	// ErrMissingSource reports a branch creation source with neither branch nor commit.
	ErrMissingSource = errors.New("branch source is required")
	// ErrAmbiguousSource reports a source that names both a branch and commit.
	ErrAmbiguousSource = errors.New("branch source must identify a branch or commit, not both")
	// ErrSourceNotFound reports a requested branch-creation source that does not exist.
	ErrSourceNotFound = errors.New("branch source not found")
	// ErrAlreadyExists reports an attempt to create an existing branch.
	ErrAlreadyExists = errors.New("branch already exists")
	// ErrNotFound reports a requested branch that does not exist.
	ErrNotFound = errors.New("branch not found")
	// ErrDefaultProtected reports an attempt to delete the default branch.
	ErrDefaultProtected = errors.New("default branch cannot be deleted")
	// ErrActiveProtected reports an attempt to delete the active branch.
	ErrActiveProtected = errors.New("active branch cannot be deleted")
)

// Source identifies the existing branch or commit from which to create a branch.
type Source struct {
	// Branch identifies an existing branch source.
	Branch string `json:"branch,omitempty"`
	// Commit identifies an existing commit source.
	Commit string `json:"commit,omitempty"`
}

// CreateRequest describes a new branch and its required source.
type CreateRequest struct {
	// Name is the new branch name.
	Name string `json:"name"`
	// Source identifies the branch or commit from which to create Name.
	Source Source `json:"source"`
}

// CreateResult identifies the created branch and its initial commit.
type CreateResult struct {
	// Name is the created branch name.
	Name string `json:"name"`
	// Commit is the source commit assigned to Name.
	Commit string `json:"commit"`
}

// ListResult contains lexically ordered branch names.
type ListResult struct {
	// Branches is the list of branch names.
	Branches []string `json:"branches"`
}

// DeleteRequest identifies a branch to delete.
type DeleteRequest struct {
	// Name is the branch name.
	Name string `json:"name"`
}

// DeleteResult identifies the deleted branch.
type DeleteResult struct {
	// Name is the deleted branch name.
	Name string `json:"name"`
}

// SwitchRequest identifies the branch to make active.
type SwitchRequest struct {
	// Name is the branch name.
	Name string `json:"name"`
}

// SwitchResult identifies the active branch after a successful switch.
type SwitchResult struct {
	// ActiveBranch is the branch made active.
	ActiveBranch string `json:"activeBranch"`
}

// Store provides the atomic persistence operation required for branch creation.
type Store interface {
	// CreateBranch atomically creates a branch at its validated source.
	CreateBranch(name string, source Source) (CreateResult, error)
	// ListBranches returns the repository's branch names.
	ListBranches() (ListResult, error)
	// DeleteBranch atomically removes a branch when repository policy permits.
	DeleteBranch(name string) (DeleteResult, error)
	// SwitchBranch atomically selects an existing branch as active.
	SwitchBranch(name string) (SwitchResult, error)
}

// Service validates branch lifecycle requests and delegates durable operations to Store.
type Service struct {
	store Store
}

// NewService returns a branch service backed by store.
func NewService(store Store) Service {
	return Service{store: store}
}

// Create validates request source, honors cancellation, and creates the branch through Store.
func (s Service) Create(ctx context.Context, request CreateRequest) (CreateResult, error) {
	if err := ctx.Err(); err != nil {
		return CreateResult{}, err
	}
	if err := ValidateSource(request.Source); err != nil {
		return CreateResult{}, err
	}
	return s.store.CreateBranch(request.Name, request.Source)
}

// List honors cancellation and returns branches from Store.
func (s Service) List(ctx context.Context) (ListResult, error) {
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}
	return s.store.ListBranches()
}

// Delete honors cancellation and delegates deletion to Store.
func (s Service) Delete(ctx context.Context, request DeleteRequest) (DeleteResult, error) {
	if err := ctx.Err(); err != nil {
		return DeleteResult{}, err
	}
	return s.store.DeleteBranch(request.Name)
}

// Switch honors cancellation and delegates branch activation to Store.
func (s Service) Switch(ctx context.Context, request SwitchRequest) (SwitchResult, error) {
	if err := ctx.Err(); err != nil {
		return SwitchResult{}, err
	}
	return s.store.SwitchBranch(request.Name)
}

// ValidateSource requires source to identify exactly one branch or commit.
func ValidateSource(source Source) error {
	switch {
	case source.Branch == "" && source.Commit == "":
		return ErrMissingSource
	case source.Branch != "" && source.Commit != "":
		return ErrAmbiguousSource
	default:
		return nil
	}
}
