package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// healthzTimeout bounds a single /healthz probe so a stalled or unreachable
// Rack endpoint never hangs the CLI.
const healthzTimeout = 5 * time.Second

// healthzResponse is the subset of a Rack /healthz body this client
// understands. The graphcontract field itself, and every version field
// within it, is optional: an older or not-yet-updated Rack server may omit
// them entirely, and that must be handled gracefully rather than treated as
// an error.
type healthzResponse struct {
	Status        string                    `json:"status,omitempty"`
	Graphcontract *healthzGraphcontractInfo `json:"graphcontract,omitempty"`
}

// healthzGraphcontractInfo is the graphcontract version block nested under
// a Rack /healthz response, e.g.:
//
//	{"status": "healthy", "graphcontract": {"packFormatVersion": 2, ...}}
type healthzGraphcontractInfo struct {
	PackFormatVersion         *uint32 `json:"packFormatVersion,omitempty"`
	PackIndexFormatVersion    *uint32 `json:"packIndexFormatVersion,omitempty"`
	PackManifestFormatVersion *uint32 `json:"packManifestFormatVersion,omitempty"`
}

// CorrelationIDHeader is the HTTP header the CLI attaches to every outbound
// Rack request, and that Rack's audit log records, so an operator can
// correlate a single CLI invocation's requests (healthz, push, pull, ...)
// with the audit events it produced remotely.
const CorrelationIDHeader = "X-Correlation-Id"

// Client performs HTTP calls against a configured Rack endpoint.
type Client struct {
	HTTPClient *http.Client
	// CorrelationID is sent as CorrelationIDHeader on every request this
	// Client issues. Callers should construct one Client per CLI command
	// invocation (via NewClient) and reuse it for every remote call that
	// invocation makes, so Rack's audit log can be correlated end to end.
	CorrelationID string
}

// NewClient returns a Client with a bounded default timeout and a freshly
// generated CorrelationID.
func NewClient() *Client {
	return &Client{HTTPClient: &http.Client{Timeout: healthzTimeout}, CorrelationID: newCorrelationID()}
}

// newCorrelationID returns a new random UUIDv4 string for use as a Client's
// CorrelationID.
func newCorrelationID() string {
	return uuid.NewString()
}

// setCorrelationHeader attaches c's CorrelationID to request, generating and
// persisting one onto c if the Client was constructed without NewClient
// (e.g. a bare Client{} in a test) so every request that Client issues
// shares the same correlation ID, and c.CorrelationID always reflects what
// was actually sent.
func (c *Client) setCorrelationHeader(request *http.Request) {
	if c.CorrelationID == "" {
		c.CorrelationID = newCorrelationID()
	}
	request.Header.Set(CorrelationIDHeader, c.CorrelationID)
}

// RackError is the decoded form of Rack's JSON error envelope:
//
//	{"error": "...", "message": "...", "correlationId": "...", "currentHead": "..."}
//
// Code is a stable, machine-readable identifier; Message is a human-readable
// description; CorrelationID lets an operator match this failure against
// Rack's audit log; CurrentHead, when present, is the branch's actual wire
// head commit ID (populated on conflict/non-fast-forward responses).
type RackError struct {
	Code          string `json:"error"`
	Message       string `json:"message"`
	CorrelationID string `json:"correlationId,omitempty"`
	CurrentHead   string `json:"currentHead,omitempty"`
}

// Error implements the error interface, preferring the human-readable
// message but falling back to the machine-readable code.
func (e *RackError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return "rack returned an error"
}

// decodeRackError best-effort decodes a non-2xx Rack HTTP response body as a
// RackError. A malformed or empty body simply yields a RackError with empty
// fields rather than an error, since the HTTP status code alone is still
// meaningful to callers.
func decodeRackError(body io.Reader) RackError {
	var envelope RackError
	_ = json.NewDecoder(body).Decode(&envelope)
	return envelope
}

// decodeRackErrorFromResponse decodes response's body as a RackError and
// redacts credential from its message, so a Rack error that happens to echo
// back a caller-supplied credential never leaks it further.
func decodeRackErrorFromResponse(response *http.Response, credential string) *RackError {
	envelope := decodeRackError(response.Body)
	envelope.Message = Redact(envelope.Message, credential)
	return &envelope
}

// setHeaders attaches authentication credentials, tenant ID header, and correlation ID to request.
func (c *Client) setHeaders(request *http.Request, authMode AuthMode, credential, tenantID string) {
	if credential != "" {
		if authMode == AuthModeAPIKey {
			request.Header.Set("X-Api-Key", credential)
		} else {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
	}
	if tenantID != "" {
		request.Header.Set("X-Tenant-ID", tenantID)
	}
	c.setCorrelationHeader(request)
}

func resourceBase(endpoint string, cfg Config) string {
	base := strings.TrimRight(endpoint, "/")
	if cfg.WorkspaceID != "" {
		return base + "/api/v1/workspaces/" + cfg.WorkspaceID
	}
	return base + "/api/v1/repos/" + cfg.RepoID
}

// fetchHealthz calls GET {endpoint}/healthz and decodes its JSON body.
// credential, when non-empty, is attached as an Authorization header per
// authMode; it is never logged or included in returned errors.
func (c *Client) fetchHealthz(ctx context.Context, endpoint string, authMode AuthMode, credential, tenantID string) (healthzResponse, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: healthzTimeout}
	}
	target := strings.TrimRight(endpoint, "/") + "/healthz"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return healthzResponse{}, fmt.Errorf("build healthz request: %w", err)
	}
	c.setHeaders(request, authMode, credential, tenantID)
	response, err := client.Do(request)
	if err != nil {
		return healthzResponse{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		rackErr := decodeRackErrorFromResponse(response, credential)
		if rackErr.Message == "" {
			rackErr.Message = fmt.Sprintf("healthz returned status %d", response.StatusCode)
		}
		return healthzResponse{}, fmt.Errorf("%w: %w", ErrRemoteUnreachable, rackErr)
	}
	var decoded healthzResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return healthzResponse{}, fmt.Errorf("%w: decode healthz response: %w", ErrRemoteUnreachable, err)
	}
	return decoded, nil
}

// ErrRemoteUnreachable reports that the configured Rack endpoint could not be
// reached or returned an unusable response.
var ErrRemoteUnreachable = errors.New("rack remote is unreachable")
