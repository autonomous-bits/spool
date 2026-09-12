package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemote_AssetNegotiateUploadStream(t *testing.T) {
	t.Parallel()

	hash1 := "1111111111111111111111111111111111111111111111111111111111111111"
	hash2 := "2222222222222222222222222222222222222222222222222222222222222222"
	payload1 := []byte("content of asset 1")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/repos/repo-1/assets/negotiate":
			var req AssetNegotiationRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(AssetNegotiationResult{
				Missing:          []string{hash1},
				Existing:         []string{hash2},
				UploadAuthorized: true,
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/repos/repo-1/assets/blobs/"+hash1:
			data, _ := io.ReadAll(r.Body)
			if !bytes.Equal(data, payload1) {
				http.Error(w, "bad payload", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repos/repo-1/assets/blobs/"+hash1:
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload1)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := Config{
		Endpoint: server.URL,
		RepoID:   "repo-1",
		AuthMode: AuthModeBearer,
	}
	client := NewClient()

	// 1. Test NegotiateAssets
	negRes, err := NegotiateAssets(context.Background(), client, cfg, "secret-token", AssetNegotiationRequest{
		Hashes: []string{hash1, hash2},
	})
	if err != nil {
		t.Fatalf("NegotiateAssets: %v", err)
	}
	if len(negRes.Missing) != 1 || negRes.Missing[0] != hash1 {
		t.Fatalf("unexpected missing: %v", negRes.Missing)
	}

	// 2. Test UploadAsset
	if err := UploadAsset(context.Background(), client, cfg, "secret-token", hash1, "text/plain", bytes.NewReader(payload1)); err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}

	// 3. Test StreamAsset
	rc, size, mimeType, err := StreamAsset(context.Background(), client, cfg, "secret-token", hash1)
	if err != nil {
		t.Fatalf("StreamAsset: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if !bytes.Equal(data, payload1) {
		t.Fatalf("streamed data mismatch: got %q, want %q", string(data), string(payload1))
	}
	if mimeType != "text/plain" {
		t.Fatalf("mimeType = %q, want text/plain", mimeType)
	}
	_ = size
}
