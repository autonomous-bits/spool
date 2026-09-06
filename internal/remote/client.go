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

// Client performs read-only HTTP calls against a configured Rack endpoint.
type Client struct {
	HTTPClient *http.Client
}

// NewClient returns a Client with a bounded default timeout.
func NewClient() *Client {
	return &Client{HTTPClient: &http.Client{Timeout: healthzTimeout}}
}

// fetchHealthz calls GET {endpoint}/healthz and decodes its JSON body.
// credential, when non-empty, is attached as an Authorization header per
// authMode; it is never logged or included in returned errors.
func (c *Client) fetchHealthz(ctx context.Context, endpoint string, authMode AuthMode, credential string) (healthzResponse, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: healthzTimeout}
	}
	target := strings.TrimRight(endpoint, "/") + "/healthz"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return healthzResponse{}, fmt.Errorf("build healthz request: %w", err)
	}
	if credential != "" {
		if authMode == AuthModeAPIKey {
			request.Header.Set("X-Api-Key", credential)
		} else {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		return healthzResponse{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return healthzResponse{}, fmt.Errorf("%w: healthz returned status %d", ErrRemoteUnreachable, response.StatusCode)
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
