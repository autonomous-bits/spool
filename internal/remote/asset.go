package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const assetTimeout = 60 * time.Second

// AssetNegotiationRequest specifies candidate asset hashes and optional sizes.
type AssetNegotiationRequest struct {
	Hashes []string         `json:"hashes"`
	Sizes  map[string]int64 `json:"sizes,omitempty"`
}

// AssetNegotiationResult is returned by Spool Rack during pre-flight push negotiation.
type AssetNegotiationResult struct {
	Missing          []string          `json:"missing"`
	Existing         []string          `json:"existing"`
	UploadAuthorized bool              `json:"uploadAuthorized"`
	Tokens           map[string]string `json:"tokens,omitempty"`
}

// NegotiateAssets executes pre-flight negotiation with Spool Rack to identify missing asset blobs.
func NegotiateAssets(ctx context.Context, client *Client, cfg Config, credential string, req AssetNegotiationRequest) (AssetNegotiationResult, error) {
	if client == nil {
		client = NewClient()
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: assetTimeout}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return AssetNegotiationResult{}, fmt.Errorf("marshal negotiation request: %w", err)
	}

	target := resourceBase(cfg.Endpoint, cfg) + "/assets/negotiate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return AssetNegotiationResult{}, fmt.Errorf("build negotiation request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client.setHeaders(httpReq, cfg.AuthMode, credential, cfg.TenantID)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return AssetNegotiationResult{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		rackErr := decodeRackErrorFromResponse(resp, credential)
		if rackErr.Message == "" {
			rackErr.Message = fmt.Sprintf("asset negotiation returned status %d", resp.StatusCode)
		}
		return AssetNegotiationResult{}, fmt.Errorf("%w: %s", ErrPushRejected, rackErr.Message)
	}

	var result AssetNegotiationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return AssetNegotiationResult{}, fmt.Errorf("%w: decode negotiation response: %w", ErrRemoteUnreachable, err)
	}
	return result, nil
}

// UploadAsset streams an asset blob to Spool Rack.
func UploadAsset(ctx context.Context, client *Client, cfg Config, credential, hash, contentType string, r io.Reader) error {
	if client == nil {
		client = NewClient()
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: assetTimeout}
	}

	target := resourceBase(cfg.Endpoint, cfg) + "/assets/blobs/" + hash
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut, target, r)
	if err != nil {
		return fmt.Errorf("build asset upload request: %w", err)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	httpReq.Header.Set("Content-Type", contentType)
	client.setHeaders(httpReq, cfg.AuthMode, credential, cfg.TenantID)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		rackErr := decodeRackErrorFromResponse(resp, credential)
		if rackErr.Message == "" {
			rackErr.Message = fmt.Sprintf("asset upload returned status %d", resp.StatusCode)
		}
		return fmt.Errorf("%w: %s", ErrPushRejected, rackErr.Message)
	}
	return nil
}

// StreamAsset fetches an asset blob from Spool Rack on demand.
func StreamAsset(ctx context.Context, client *Client, cfg Config, credential, hash string) (io.ReadCloser, int64, string, error) {
	if client == nil {
		client = NewClient()
	}
	httpClient := client.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: assetTimeout}
	}

	target := resourceBase(cfg.Endpoint, cfg) + "/assets/blobs/" + hash
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, "", fmt.Errorf("build asset stream request: %w", err)
	}
	client.setHeaders(httpReq, cfg.AuthMode, credential, cfg.TenantID)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, "", fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, 0, "", errors.New("asset not found on remote")
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		_ = resp.Body.Close()
		rackErr := decodeRackErrorFromResponse(resp, credential)
		return nil, 0, "", fmt.Errorf("remote asset retrieval failed (status %d): %s", resp.StatusCode, rackErr.Message)
	}

	size := resp.ContentLength
	if size <= 0 {
		if cl := resp.Header.Get("Content-Length"); cl != "" {
			size, _ = strconv.ParseInt(strings.TrimSpace(cl), 10, 64)
		}
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return resp.Body, size, contentType, nil
}
