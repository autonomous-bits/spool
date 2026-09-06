package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/autonomous-bits/spool/graphcontract"
)

// pushTimeout bounds a single push HTTP call. Packs can be large, so this is
// considerably more generous than healthzTimeout.
const pushTimeout = 60 * time.Second

// PushCommitRecord is one commit to register with Rack as part of a push,
// matching Rack's expected `metadata.commits[]` wire shape byte-for-byte.
type PushCommitRecord struct {
	ID     string               `json:"id"`
	Commit graphcontract.Commit `json:"commit"`
}

// PushRequest describes a native push to send to a Rack remote. Branch,
// BaseCommit, TargetCommit, Commits, PackHash, and PackFormat become the
// JSON `metadata` multipart part; PackData becomes the binary `pack` part.
// Callers (the CLI command layer) build this from a
// repository.PushPack.
type PushRequest struct {
	Branch       string
	BaseCommit   string
	TargetCommit string
	Commits      []PushCommitRecord
	PackHash     string
	PackFormat   uint32
	PackData     []byte
}

// PushResult is Rack's success response to a push: the branch that advanced
// and the wire commit ID it now points at.
type PushResult struct {
	Branch     string `json:"branch"`
	HeadCommit string `json:"headCommit"`
}

// NonFastForwardError reports that Rack rejected a push because the
// supplied BaseCommit is no longer the branch's actual head. ActualHead is
// Rack's current wire head commit ID for the branch; Guidance is a
// human-readable message from Rack explaining the rejection.
type NonFastForwardError struct {
	ActualHead string
	Guidance   string
}

func (e *NonFastForwardError) Error() string {
	if e.Guidance != "" {
		return e.Guidance
	}
	return fmt.Sprintf("push rejected: remote head is %s", e.ActualHead)
}

// ErrPushRejected reports that Rack rejected a push for a reason other than
// a non-fast-forward base (e.g. a malformed pack or invalid metadata).
var ErrPushRejected = errors.New("rack rejected the push")

// pushErrorEnvelope mirrors Rack's JSON error body shape:
// {"error": "...", "message": "...", "currentHead": "..."}.
type pushErrorEnvelope struct {
	Error       string `json:"error"`
	Message     string `json:"message"`
	CurrentHead string `json:"currentHead,omitempty"`
}

// push POSTs a multipart/form-data push request (a JSON `metadata` part
// followed by a binary `pack` part, matching the part order Rack's gateway
// requires) to {endpoint}/api/v1/repos/{repoID}/push.
func (c *Client) push(ctx context.Context, endpoint, repoID string, authMode AuthMode, credential string, req PushRequest) (PushResult, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: pushTimeout}
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	contentType := writer.FormDataContentType()

	metadata := struct {
		Branch       string             `json:"branch"`
		BaseCommit   string             `json:"baseCommit"`
		TargetCommit string             `json:"targetCommit"`
		PackHash     string             `json:"packHash"`
		PackFormat   uint32             `json:"packFormat,omitempty"`
		Commits      []PushCommitRecord `json:"commits,omitempty"`
	}{
		Branch:       req.Branch,
		BaseCommit:   req.BaseCommit,
		TargetCommit: req.TargetCommit,
		PackHash:     req.PackHash,
		PackFormat:   req.PackFormat,
		Commits:      req.Commits,
	}

	// The multipart body is streamed directly into the HTTP request via a
	// pipe rather than buffered into memory first, so a large pack does not
	// require holding two full copies of it in RAM (req.PackData plus a
	// buffered request body). Any build failure here (or the transport
	// abandoning the body on its own connection failure) closes the pipe
	// with an error, which client.Do below surfaces as its own error.
	go func() {
		err := func() error {
			metadataPart, err := writer.CreateFormField("metadata")
			if err != nil {
				return fmt.Errorf("build push request: %w", err)
			}
			if err := json.NewEncoder(metadataPart).Encode(metadata); err != nil {
				return fmt.Errorf("encode push metadata: %w", err)
			}
			packPart, err := writer.CreateFormFile("pack", "pack.cbor")
			if err != nil {
				return fmt.Errorf("build push request: %w", err)
			}
			if _, err := packPart.Write(req.PackData); err != nil {
				return fmt.Errorf("write push pack: %w", err)
			}
			return writer.Close()
		}()
		_ = pipeWriter.CloseWithError(err)
	}()

	target := strings.TrimRight(endpoint, "/") + "/api/v1/repos/" + repoID + "/push"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, pipeReader)
	if err != nil {
		return PushResult{}, fmt.Errorf("build push request: %w", err)
	}
	request.Header.Set("Content-Type", contentType)
	if credential != "" {
		if authMode == AuthModeAPIKey {
			request.Header.Set("X-Api-Key", credential)
		} else {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
	}

	response, err := client.Do(request)
	if err != nil {
		return PushResult{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusOK:
		var result PushResult
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			return PushResult{}, fmt.Errorf("%w: decode push response: %w", ErrRemoteUnreachable, err)
		}
		return result, nil
	case http.StatusConflict:
		var envelope pushErrorEnvelope
		_ = json.NewDecoder(response.Body).Decode(&envelope)
		return PushResult{}, &NonFastForwardError{ActualHead: envelope.CurrentHead, Guidance: envelope.Message}
	default:
		var envelope pushErrorEnvelope
		_ = json.NewDecoder(response.Body).Decode(&envelope)
		message := envelope.Message
		if message == "" {
			message = fmt.Sprintf("push returned status %d", response.StatusCode)
		}
		return PushResult{}, fmt.Errorf("%w: %s", ErrPushRejected, Redact(message, credential))
	}
}

// Push sends req to cfg.Endpoint/cfg.RepoID and returns Rack's response.
// credential, when non-empty, is attached per cfg.AuthMode. A non-fast-
// forward rejection is returned as a *NonFastForwardError (use errors.As);
// any other rejection is wrapped in ErrPushRejected; an unreachable or
// malformed-response remote is wrapped in ErrRemoteUnreachable.
func Push(ctx context.Context, client *Client, cfg Config, credential string, req PushRequest) (PushResult, error) {
	if client == nil {
		client = NewClient()
	}
	return client.push(ctx, cfg.Endpoint, cfg.RepoID, cfg.AuthMode, credential, req)
}
