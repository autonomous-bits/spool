package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// branchTimeout bounds a single remote branch lifecycle HTTP call. These
// calls exchange small JSON bodies only (no pack data), so this is far
// tighter than pushTimeout/pullTimeout.
const branchTimeout = 15 * time.Second

// BranchCreateRequest describes a new remote branch to create. Exactly one
// of SourceBranch or SourceCommit must be set, matching the local
// branch.Source contract.
type BranchCreateRequest struct {
	Name         string `json:"name"`
	SourceBranch string `json:"sourceBranch,omitempty"`
	SourceCommit string `json:"sourceCommit,omitempty"`
}

// BranchResult identifies a remote branch and the wire commit ID it
// currently points at.
type BranchResult struct {
	Name       string `json:"name"`
	HeadCommit string `json:"headCommit"`
}

// BranchListEntry is one branch in a BranchListResult.
type BranchListEntry struct {
	Name       string `json:"name"`
	HeadCommit string `json:"headCommit"`
	Default    bool   `json:"default,omitempty"`
}

// BranchListResult is Rack's response to a remote branch list request.
type BranchListResult struct {
	Branches []BranchListEntry `json:"branches"`
}

var (
	// ErrBranchAlreadyExists reports that Rack rejected a branch creation
	// because a branch with that name already exists.
	ErrBranchAlreadyExists = errors.New("rack branch already exists")
	// ErrBranchSourceNotFound reports that Rack rejected a branch creation
	// because its named source branch or commit does not exist.
	ErrBranchSourceNotFound = errors.New("rack branch source not found")
	// ErrBranchNotFound reports that Rack has no such branch for the
	// configured repository.
	ErrBranchNotFound = errors.New("rack remote has no such branch")
	// ErrBranchRejected reports that Rack rejected a branch lifecycle
	// request for a reason other than the typed cases above.
	ErrBranchRejected = errors.New("rack rejected the branch request")
)

// ProtectedBranchError reports that Rack refused to delete a branch because
// it is the repository's protected default branch. CorrelationID lets an
// operator match this failure against Rack's audit log.
type ProtectedBranchError struct {
	Guidance      string
	CorrelationID string
}

func (e *ProtectedBranchError) Error() string {
	if e.Guidance != "" {
		return e.Guidance
	}
	return "branch is protected and cannot be deleted"
}

// branchesEndpoint returns {endpoint}/api/v1/repos/{repoID}/branches.
func branchesEndpoint(endpoint, repoID string) string {
	return strings.TrimRight(endpoint, "/") + "/api/v1/repos/" + repoID + "/branches"
}

func (c *Client) branchHTTPClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: branchTimeout}
}

func (c *Client) newBranchRequest(ctx context.Context, method, target string, authMode AuthMode, credential string, body []byte) (*http.Request, error) {
	var reader *strings.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	var request *http.Request
	var err error
	if reader != nil {
		request, err = http.NewRequestWithContext(ctx, method, target, reader)
	} else {
		request, err = http.NewRequestWithContext(ctx, method, target, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", method, err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if credential != "" {
		if authMode == AuthModeAPIKey {
			request.Header.Set("X-Api-Key", credential)
		} else {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
	}
	c.setCorrelationHeader(request)
	return request, nil
}

// createBranch calls POST {endpoint}/api/v1/repos/{repoID}/branches.
func (c *Client) createBranch(ctx context.Context, endpoint, repoID string, authMode AuthMode, credential string, req BranchCreateRequest) (BranchResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return BranchResult{}, fmt.Errorf("encode branch create request: %w", err)
	}
	request, err := c.newBranchRequest(ctx, http.MethodPost, branchesEndpoint(endpoint, repoID), authMode, credential, body)
	if err != nil {
		return BranchResult{}, err
	}
	response, err := c.branchHTTPClient().Do(request)
	if err != nil {
		return BranchResult{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var result BranchResult
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			return BranchResult{}, fmt.Errorf("%w: decode branch create response: %w", ErrRemoteUnreachable, err)
		}
		return result, nil
	case http.StatusConflict:
		rackErr := decodeRackErrorFromResponse(response, credential)
		return BranchResult{}, fmt.Errorf("%w: %w", ErrBranchAlreadyExists, rackErr)
	case http.StatusNotFound:
		rackErr := decodeRackErrorFromResponse(response, credential)
		return BranchResult{}, fmt.Errorf("%w: %w", ErrBranchSourceNotFound, rackErr)
	default:
		rackErr := decodeRackErrorFromResponse(response, credential)
		if rackErr.Message == "" {
			rackErr.Message = fmt.Sprintf("branch create returned status %d", response.StatusCode)
		}
		return BranchResult{}, fmt.Errorf("%w: %w", ErrBranchRejected, rackErr)
	}
}

// listBranches calls GET {endpoint}/api/v1/repos/{repoID}/branches.
func (c *Client) listBranches(ctx context.Context, endpoint, repoID string, authMode AuthMode, credential string) (BranchListResult, error) {
	request, err := c.newBranchRequest(ctx, http.MethodGet, branchesEndpoint(endpoint, repoID), authMode, credential, nil)
	if err != nil {
		return BranchListResult{}, err
	}
	response, err := c.branchHTTPClient().Do(request)
	if err != nil {
		return BranchListResult{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		rackErr := decodeRackErrorFromResponse(response, credential)
		if rackErr.Message == "" {
			rackErr.Message = fmt.Sprintf("branch list returned status %d", response.StatusCode)
		}
		return BranchListResult{}, fmt.Errorf("%w: %w", ErrBranchRejected, rackErr)
	}
	var result BranchListResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return BranchListResult{}, fmt.Errorf("%w: decode branch list response: %w", ErrRemoteUnreachable, err)
	}
	return result, nil
}

// defaultBranch calls GET {endpoint}/api/v1/repos/{repoID}/branches/default.
func (c *Client) defaultBranch(ctx context.Context, endpoint, repoID string, authMode AuthMode, credential string) (BranchResult, error) {
	request, err := c.newBranchRequest(ctx, http.MethodGet, branchesEndpoint(endpoint, repoID)+"/default", authMode, credential, nil)
	if err != nil {
		return BranchResult{}, err
	}
	response, err := c.branchHTTPClient().Do(request)
	if err != nil {
		return BranchResult{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		rackErr := decodeRackErrorFromResponse(response, credential)
		if rackErr.Message == "" {
			rackErr.Message = fmt.Sprintf("branch default returned status %d", response.StatusCode)
		}
		return BranchResult{}, fmt.Errorf("%w: %w", ErrBranchRejected, rackErr)
	}
	var result BranchResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return BranchResult{}, fmt.Errorf("%w: decode branch default response: %w", ErrRemoteUnreachable, err)
	}
	return result, nil
}

// deleteBranch calls DELETE {endpoint}/api/v1/repos/{repoID}/branches/{name}.
func (c *Client) deleteBranch(ctx context.Context, endpoint, repoID string, authMode AuthMode, credential string, name string) error {
	request, err := c.newBranchRequest(ctx, http.MethodDelete, branchesEndpoint(endpoint, repoID)+"/"+strings.TrimPrefix(name, "/"), authMode, credential, nil)
	if err != nil {
		return err
	}
	response, err := c.branchHTTPClient().Do(request)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusConflict:
		rackErr := decodeRackErrorFromResponse(response, credential)
		return &ProtectedBranchError{Guidance: rackErr.Message, CorrelationID: rackErr.CorrelationID}
	case http.StatusNotFound:
		rackErr := decodeRackErrorFromResponse(response, credential)
		return fmt.Errorf("%w: %w", ErrBranchNotFound, rackErr)
	default:
		rackErr := decodeRackErrorFromResponse(response, credential)
		if rackErr.Message == "" {
			rackErr.Message = fmt.Sprintf("branch delete returned status %d", response.StatusCode)
		}
		return fmt.Errorf("%w: %w", ErrBranchRejected, rackErr)
	}
}

// CreateBranch creates a remote branch named req.Name from exactly one of
// req.SourceBranch or req.SourceCommit against cfg.Endpoint/cfg.RepoID.
// credential, when non-empty, is attached per cfg.AuthMode. An existing
// branch of the same name is wrapped in ErrBranchAlreadyExists; a missing
// source is wrapped in ErrBranchSourceNotFound; any other rejection is
// wrapped in ErrBranchRejected; an unreachable or malformed-response remote
// is wrapped in ErrRemoteUnreachable.
func CreateBranch(ctx context.Context, client *Client, cfg Config, credential string, req BranchCreateRequest) (BranchResult, error) {
	if client == nil {
		client = NewClient()
	}
	return client.createBranch(ctx, cfg.Endpoint, cfg.RepoID, cfg.AuthMode, credential, req)
}

// ListBranches lists every remote branch for cfg.Endpoint/cfg.RepoID.
// credential, when non-empty, is attached per cfg.AuthMode.
func ListBranches(ctx context.Context, client *Client, cfg Config, credential string) (BranchListResult, error) {
	if client == nil {
		client = NewClient()
	}
	return client.listBranches(ctx, cfg.Endpoint, cfg.RepoID, cfg.AuthMode, credential)
}

// DefaultBranch discovers the default branch for cfg.Endpoint/cfg.RepoID.
// credential, when non-empty, is attached per cfg.AuthMode.
func DefaultBranch(ctx context.Context, client *Client, cfg Config, credential string) (BranchResult, error) {
	if client == nil {
		client = NewClient()
	}
	return client.defaultBranch(ctx, cfg.Endpoint, cfg.RepoID, cfg.AuthMode, credential)
}

// DeleteBranch deletes the named remote branch from cfg.Endpoint/cfg.RepoID.
// credential, when non-empty, is attached per cfg.AuthMode. An attempt to
// delete the repository's protected default branch is returned as a
// *ProtectedBranchError (use errors.As); a missing branch is wrapped in
// ErrBranchNotFound; any other rejection is wrapped in ErrBranchRejected.
func DeleteBranch(ctx context.Context, client *Client, cfg Config, credential string, name string) error {
	if client == nil {
		client = NewClient()
	}
	return client.deleteBranch(ctx, cfg.Endpoint, cfg.RepoID, cfg.AuthMode, credential, name)
}
