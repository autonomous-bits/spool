package remote

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// buildTestPullEnvelope constructs a valid, canonically encoded v2 pull
// envelope carrying packs verbatim (their content need not be valid
// PackFrameV2 CBOR: the remote package only hash-verifies and returns pack
// boundaries, leaving PackFrameV2 decoding to internal/repository).
func buildTestPullEnvelope(t *testing.T, head string, packs [][]byte) []byte {
	t.Helper()
	return buildTestPullEnvelopeWithFormat(t, head, packs, 2)
}

// buildTestPullEnvelopeWithFormat is buildTestPullEnvelope but lets the
// caller override every pack's declared manifest Format field, to exercise
// pack-format validation.
func buildTestPullEnvelopeWithFormat(t *testing.T, head string, packs [][]byte, packFormat uint32) []byte {
	t.Helper()
	manifest := pullManifestV2{
		Version: pullEnvelopeFormatV2,
		Head:    head,
		Packs:   make([]pullPackManifestV2, len(packs)),
	}
	for i, pack := range packs {
		manifest.Packs[i] = pullPackManifestV2{
			Hash:   pullContentID(pack),
			Format: packFormat,
			Length: uint64(len(pack)),
		}
	}
	manifestData, err := pullCanonicalCBOR.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	var out bytes.Buffer
	var header [pullEnvelopeHeaderSize]byte
	copy(header[:4], pullEnvelopeMagic)
	binary.BigEndian.PutUint32(header[4:8], pullEnvelopeFormatV2)
	binary.BigEndian.PutUint64(header[8:], uint64(len(manifestData)))
	out.Write(header[:])
	out.Write(manifestData)
	for _, pack := range packs {
		out.Write(pack)
	}
	return out.Bytes()
}

func writeZstdPullResponse(t *testing.T, w http.ResponseWriter, envelope []byte) {
	t.Helper()
	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatalf("new zstd writer: %v", err)
	}
	if _, err := encoder.Write(envelope); err != nil {
		t.Fatalf("compress envelope: %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("close zstd writer: %v", err)
	}
	w.Header().Set("Content-Type", "application/vnd.spool-rack.pull-envelope")
	w.Header().Set("Content-Encoding", "zstd")
	w.Header().Set("X-Spool-Pull-Format", "2")
	w.Header().Set("X-Spool-Head-Commit", "unused-see-manifest-head")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(compressed.Bytes())
}

func TestPullUpToDateReturnsHeadFromHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("branch"); got != "main" {
			t.Fatalf("branch = %q", got)
		}
		if got := r.URL.Query().Get("knownCommit"); got != "abc123" {
			t.Fatalf("knownCommit = %q", got)
		}
		w.Header().Set("X-Spool-Head-Commit", "abc123")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	result, err := Pull(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "secret", "main", "abc123")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if !result.UpToDate || result.HeadCommit != "abc123" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPullSuccessDecodesPacksInOrder(t *testing.T) {
	packs := [][]byte{[]byte("pack-one"), []byte("pack-two")}
	envelope := buildTestPullEnvelope(t, "deadbeef", packs)

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeZstdPullResponse(t, w, envelope)
	}))
	defer server.Close()

	result, err := Pull(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "main", "")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if result.UpToDate {
		t.Fatal("result.UpToDate = true, want false")
	}
	if result.HeadCommit != "deadbeef" {
		t.Fatalf("HeadCommit = %q", result.HeadCommit)
	}
	if len(result.Packs) != 2 || string(result.Packs[0]) != "pack-one" || string(result.Packs[1]) != "pack-two" {
		t.Fatalf("Packs = %#v", result.Packs)
	}
	if gotPath != "/api/v1/repos/acme/pull" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestPullTamperedPackHashRejected(t *testing.T) {
	packs := [][]byte{[]byte("pack-one")}
	envelope := buildTestPullEnvelope(t, "deadbeef", packs)
	// Flip a byte inside the pack payload (after the header+manifest) so its
	// content no longer matches the manifest's declared hash.
	envelope[len(envelope)-1] ^= 0xFF

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeZstdPullResponse(t, w, envelope)
	}))
	defer server.Close()

	_, err := Pull(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "main", "")
	if !errors.Is(err, ErrInvalidPullEnvelope) {
		t.Fatalf("err = %v, want ErrInvalidPullEnvelope", err)
	}
}

func TestPullUnsupportedPackFormatRejected(t *testing.T) {
	packs := [][]byte{[]byte("pack-one")}
	envelope := buildTestPullEnvelopeWithFormat(t, "deadbeef", packs, 99)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeZstdPullResponse(t, w, envelope)
	}))
	defer server.Close()

	_, err := Pull(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "main", "")
	if !errors.Is(err, ErrInvalidPullEnvelope) {
		t.Fatalf("err = %v, want ErrInvalidPullEnvelope", err)
	}
}

func TestPullDivergedReturnsTypedError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":       "conflict",
			"message":     "known commit is not an ancestor of the remote branch head",
			"currentHead": "def456",
		})
	}))
	defer server.Close()

	_, err := Pull(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "main", "stale-commit")
	var diverged *DivergedError
	if !errors.As(err, &diverged) {
		t.Fatalf("err = %v, want *DivergedError", err)
	}
	if diverged.ActualHead != "def456" {
		t.Fatalf("diverged = %#v", diverged)
	}
}

func TestPullBranchNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := Pull(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "missing-branch", "")
	if !errors.Is(err, ErrPullBranchNotFound) {
		t.Fatalf("err = %v, want ErrPullBranchNotFound", err)
	}
}

func TestPullBadRequestWrapsErrPullRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "bad_request",
			"message": "branch query parameter is required",
		})
	}))
	defer server.Close()

	_, err := Pull(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "", "", "")
	if !errors.Is(err, ErrPullRejected) {
		t.Fatalf("err = %v, want ErrPullRejected", err)
	}
}

func TestPullUnreachableRemote(t *testing.T) {
	_, err := Pull(context.Background(), NewClient(), Config{Endpoint: "http://127.0.0.1:1", RepoID: "acme", AuthMode: AuthModeBearer}, "", "main", "")
	if !errors.Is(err, ErrRemoteUnreachable) {
		t.Fatalf("err = %v, want ErrRemoteUnreachable", err)
	}
}

func TestPullCredentialNeverAppearsInErrorText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "bad_request",
			"message": "credential super-secret-token was rejected",
		})
	}))
	defer server.Close()

	_, err := Pull(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "super-secret-token", "main", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret-token") {
		t.Fatalf("error %q leaked credential", err.Error())
	}
}
