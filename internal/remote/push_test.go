package remote

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/graphcontract"
)

func testPushRequest() PushRequest {
	return PushRequest{
		Branch:       "main",
		BaseCommit:   "",
		TargetCommit: "abc123",
		Commits: []PushCommitRecord{
			{ID: "abc123", Commit: graphcontract.Commit{
				Author: "alice", Message: "hello", Time: time.Unix(0, 0).UTC(),
			}},
		},
		PackHash:   "deadbeef",
		PackFormat: 2,
		PackData:   []byte("pack-bytes"),
	}
}

func decodeMultipartPush(t *testing.T, r *http.Request) (metadata map[string]any, packBytes []byte) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, err = %v", r.Header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(r.Body, params["boundary"])
	var sawMetadata bool
	for {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		switch part.FormName() {
		case "metadata":
			if err := json.NewDecoder(part).Decode(&metadata); err != nil {
				t.Fatalf("decode metadata: %v", err)
			}
			sawMetadata = true
		case "pack":
			buf := make([]byte, 0, 64)
			tmp := make([]byte, 64)
			for {
				n, err := part.Read(tmp)
				buf = append(buf, tmp[:n]...)
				if err != nil {
					break
				}
			}
			packBytes = buf
		}
		_ = part.Close()
	}
	if !sawMetadata {
		t.Fatal("request did not include a metadata part")
	}
	return metadata, packBytes
}

func TestPushSuccessDecodesResult(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		metadata, packBytes := decodeMultipartPush(t, r)
		if metadata["branch"] != "main" || metadata["targetCommit"] != "abc123" {
			t.Fatalf("metadata = %#v", metadata)
		}
		if string(packBytes) != "pack-bytes" {
			t.Fatalf("packBytes = %q", packBytes)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(PushResult{Branch: "main", HeadCommit: "abc123"})
	}))
	defer server.Close()

	result, err := Push(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "secret-token", testPushRequest())
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if result.Branch != "main" || result.HeadCommit != "abc123" {
		t.Fatalf("result = %#v", result)
	}
	if gotPath != "/api/v1/repos/acme/push" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestPushNonFastForwardReturnsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeMultipartPush(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":       "conflict",
			"message":     "branch has moved",
			"currentHead": "def456",
		})
	}))
	defer server.Close()

	_, err := Push(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", testPushRequest())
	var nffErr *NonFastForwardError
	if !errors.As(err, &nffErr) {
		t.Fatalf("err = %v, want *NonFastForwardError", err)
	}
	if nffErr.ActualHead != "def456" || nffErr.Guidance != "branch has moved" {
		t.Fatalf("nffErr = %#v", nffErr)
	}
}

func TestPushBadRequestWrapsErrPushRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeMultipartPush(t, r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "bad_request",
			"message": "packHash must be 64 lowercase hex characters",
		})
	}))
	defer server.Close()

	_, err := Push(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", testPushRequest())
	if !errors.Is(err, ErrPushRejected) {
		t.Fatalf("err = %v, want ErrPushRejected", err)
	}
}

func TestPushUnreachableRemote(t *testing.T) {
	_, err := Push(context.Background(), NewClient(), Config{Endpoint: "http://127.0.0.1:1", RepoID: "acme", AuthMode: AuthModeBearer}, "", testPushRequest())
	if !errors.Is(err, ErrRemoteUnreachable) {
		t.Fatalf("err = %v, want ErrRemoteUnreachable", err)
	}
}

func TestPushCredentialNeverAppearsInErrorText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeMultipartPush(t, r)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "bad_request",
			"message": "credential super-secret-token was rejected",
		})
	}))
	defer server.Close()

	_, err := Push(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "super-secret-token", testPushRequest())
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" {
		t.Fatal("empty error")
	}
	for _, sub := range []string{"super-secret-token"} {
		if strings.Contains(err.Error(), sub) {
			t.Fatalf("error %q leaked credential", err.Error())
		}
	}
}
