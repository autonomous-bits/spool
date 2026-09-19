package ctxgit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

// OpenPRRequest is a pull request to the protected branch of the context remote.
type OpenPRRequest struct {
	Remote string
	Head   string
	Base   string
	Title  string
	Body   string
}

// PullRequest is the result of opening a review PR.
type PullRequest struct {
	URL    string `json:"url"`
	Number int    `json:"number,omitempty"`
	Head   string `json:"head"`
	Base   string `json:"base"`
}

// PROpener opens a host pull request using stock git-host credentials (not a
// Spool-specific protocol). Tests inject a recorder.
type PROpener interface {
	OpenPR(ctx context.Context, req OpenPRRequest) (PullRequest, error)
}

// RecordingPROpener stores requests and returns a synthetic PR URL.
type RecordingPROpener struct {
	Requests []OpenPRRequest
	NextURL  string
}

// OpenPR records req and returns a fake pull request.
func (r *RecordingPROpener) OpenPR(_ context.Context, req OpenPRRequest) (PullRequest, error) {
	r.Requests = append(r.Requests, req)
	url := r.NextURL
	if url == "" {
		url = "https://example.invalid/pr/" + req.Head
	}
	return PullRequest{URL: url, Head: req.Head, Base: req.Base, Number: len(r.Requests)}, nil
}

// HostPROpener opens GitHub pull requests with GH_TOKEN/GITHUB_TOKEN or `gh`.
type HostPROpener struct {
	HTTP *http.Client
}

func defaultPROpener() PROpener {
	return HostPROpener{}
}

// OpenPR creates a GitHub PR when the remote is a GitHub URL.
func (o HostPROpener) OpenPR(ctx context.Context, req OpenPRRequest) (PullRequest, error) {
	owner, repo, ok := parseGitHubRemote(req.Remote)
	if !ok {
		if pr, err := openWithGH(ctx, req); err == nil {
			return pr, nil
		}
		return PullRequest{}, fmt.Errorf("pushed branch %q; open a pull request into %q on the git host (remote %q is not a recognized GitHub URL)", req.Head, req.Base, req.Remote)
	}
	if pr, err := o.openGitHubAPI(ctx, owner, repo, req); err == nil {
		return pr, nil
	} else if pr, ghErr := openWithGH(ctx, req); ghErr == nil {
		return pr, nil
	} else {
		return PullRequest{}, fmt.Errorf("open pull request: %v", err)
	}
}

func (o HostPROpener) openGitHubAPI(ctx context.Context, owner, repo string, req OpenPRRequest) (PullRequest, error) {
	token := strings.TrimSpace(os.Getenv("GH_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	if token == "" {
		return PullRequest{}, fmt.Errorf("GH_TOKEN/GITHUB_TOKEN is not set")
	}
	client := o.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	payload, err := json.Marshal(map[string]any{
		"title": req.Title,
		"body":  req.Body,
		"head":  req.Head,
		"base":  req.Base,
	})
	if err != nil {
		return PullRequest{}, err
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repo)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return PullRequest{}, err
	}
	httpReq.Header.Set("Accept", "application/vnd.github+json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return PullRequest{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PullRequest{}, fmt.Errorf("GitHub API %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		HTMLURL string `json:"html_url"`
		Number  int    `json:"number"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return PullRequest{}, err
	}
	return PullRequest{URL: decoded.HTMLURL, Number: decoded.Number, Head: req.Head, Base: req.Base}, nil
}

func openWithGH(ctx context.Context, req OpenPRRequest) (PullRequest, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return PullRequest{}, err
	}
	cmd := exec.CommandContext(ctx, "gh", "pr", "create", "--head", req.Head, "--base", req.Base, "--title", req.Title, "--body", req.Body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return PullRequest{}, fmt.Errorf("gh pr create: %s", strings.TrimSpace(stderr.String()))
	}
	return PullRequest{URL: strings.TrimSpace(stdout.String()), Head: req.Head, Base: req.Base}, nil
}

func parseGitHubRemote(remote string) (owner, repo string, ok bool) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", "", false
	}
	if strings.HasPrefix(remote, "git@github.com:") {
		path := strings.TrimPrefix(remote, "git@github.com:")
		path = strings.TrimSuffix(path, ".git")
		parts := strings.Split(path, "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return parts[0], parts[1], true
		}
		return "", "", false
	}
	parsed, err := url.Parse(remote)
	if err != nil {
		return "", "", false
	}
	host := strings.ToLower(parsed.Host)
	if host != "github.com" && host != "www.github.com" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[0], strings.TrimSuffix(parts[1], ".git"), true
}
