package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// CloneResult is the outcome of a successful clone HTTP call.
type CloneResult struct {
	Branch        string
	DefaultBranch string
	HeadCommit    string
	Packs         [][]byte
	Empty         bool
}

// Clone fetches the complete history for branch (or the remote default branch if empty)
// from cfg.Endpoint/cfg.WorkspaceOrRepoID().
// If the remote server does not yet support the direct clone endpoint (returns 404),
// Clone transparently falls back to discovering the default branch via DefaultBranch and
// fetching its full history via Pull.
func Clone(ctx context.Context, client *Client, cfg Config, credential string, branch string) (CloneResult, error) {
	if client == nil {
		client = NewClient()
	}
	return client.clone(ctx, cfg, credential, branch)
}

func (c *Client) clone(ctx context.Context, cfg Config, credential string, branch string) (CloneResult, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: pullTimeout}
	}

	query := url.Values{}
	if branch != "" {
		query.Set("branch", branch)
	}
	queryString := ""
	if len(query) > 0 {
		queryString = "?" + query.Encode()
	}

	target := resourceBase(cfg.Endpoint, cfg) + "/clone" + queryString
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return CloneResult{}, fmt.Errorf("build clone request: %w", err)
	}
	c.setHeaders(request, cfg.AuthMode, credential, cfg.TenantID)

	response, err := client.Do(request)
	if err != nil {
		return CloneResult{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
		if response.Header.Get("X-Spool-Empty") == "true" {
			emptyBranch := response.Header.Get("X-Spool-Branch")
			if emptyBranch == "" {
				emptyBranch = "main"
			}
			defaultBranch := response.Header.Get("X-Spool-Default-Branch")
			if defaultBranch == "" {
				defaultBranch = emptyBranch
			}
			return CloneResult{
				Branch:        emptyBranch,
				DefaultBranch: defaultBranch,
				Empty:         true,
			}, nil
		}

		pullRes, err := decodePullResponse(response)
		if err != nil {
			return CloneResult{}, err
		}

		clonedBranch := response.Header.Get("X-Spool-Branch")
		if clonedBranch == "" {
			clonedBranch = branch
		}
		if clonedBranch == "" {
			clonedBranch = "main"
		}
		defaultBranch := response.Header.Get("X-Spool-Default-Branch")
		if defaultBranch == "" {
			defaultBranch = clonedBranch
		}

		return CloneResult{
			Branch:        clonedBranch,
			DefaultBranch: defaultBranch,
			HeadCommit:    pullRes.HeadCommit,
			Packs:         pullRes.Packs,
			Empty:         false,
		}, nil

	case http.StatusNotFound:
		rackErr := decodeRackErrorFromResponse(response, credential)
		if rackErr.Message == "branch not found" {
			return CloneResult{}, fmt.Errorf("%w: %w", ErrPullBranchNotFound, rackErr)
		}

		// Fallback for servers that do not have the /clone endpoint
		targetBranch := branch
		var defBranch string
		if targetBranch == "" {
			defaultRes, err := c.defaultBranch(ctx, cfg, credential)
			if err != nil {
				return CloneResult{}, fmt.Errorf("discover default branch for clone: %w", err)
			}
			targetBranch = defaultRes.Name
			defBranch = defaultRes.Name
		} else {
			defBranch = targetBranch
		}

		pullRes, err := c.pull(ctx, cfg, credential, targetBranch, "")
		if err != nil {
			return CloneResult{}, fmt.Errorf("pull branch for clone: %w", err)
		}

		return CloneResult{
			Branch:        targetBranch,
			DefaultBranch: defBranch,
			HeadCommit:    pullRes.HeadCommit,
			Packs:         pullRes.Packs,
			Empty:         pullRes.HeadCommit == "",
		}, nil

	default:
		rackErr := decodeRackErrorFromResponse(response, credential)
		if rackErr.Message == "" {
			rackErr.Message = fmt.Sprintf("clone returned status %d", response.StatusCode)
		}
		return CloneResult{}, fmt.Errorf("%w: %w", ErrPullRejected, rackErr)
	}
}
