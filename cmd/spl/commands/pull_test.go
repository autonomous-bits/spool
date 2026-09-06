package commands

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
	"lukechampine.com/blake3"
)

// pullTestManifestPack mirrors Rack's PullPackManifestV2 wire shape.
type pullTestManifestPack struct {
	Hash   string `cbor:"1,keyasint"`
	Format uint32 `cbor:"2,keyasint"`
	Length uint64 `cbor:"3,keyasint"`
}

// pullTestManifest mirrors Rack's PullManifestV2 wire shape.
type pullTestManifest struct {
	Version uint32                 `cbor:"1,keyasint"`
	Head    string                 `cbor:"2,keyasint"`
	Packs   []pullTestManifestPack `cbor:"3,keyasint"`
}

func pullTestContentID(data []byte) string {
	sum := blake3.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// buildTestPullEnvelope constructs a valid v2 pull envelope (SRPL header +
// canonical CBOR manifest + concatenated raw packs), matching the wire
// contract internal/remote.Pull decodes.
func buildTestPullEnvelope(t *testing.T, head string, packs [][]byte) []byte {
	t.Helper()
	canonical, _ := cbor.CanonicalEncOptions().EncMode()
	manifest := pullTestManifest{Version: 2, Head: head, Packs: make([]pullTestManifestPack, len(packs))}
	for i, pack := range packs {
		manifest.Packs[i] = pullTestManifestPack{Hash: pullTestContentID(pack), Format: 2, Length: uint64(len(pack))}
	}
	manifestData, err := canonical.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	var out bytes.Buffer
	var header [16]byte
	copy(header[:4], "SRPL")
	binary.BigEndian.PutUint32(header[4:8], 2)
	binary.BigEndian.PutUint64(header[8:], uint64(len(manifestData)))
	out.Write(header[:])
	out.Write(manifestData)
	for _, pack := range packs {
		out.Write(pack)
	}
	return out.Bytes()
}

func writeTestZstdPullResponse(t *testing.T, w http.ResponseWriter, envelope []byte) {
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
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(compressed.Bytes())
}

func TestPullInstallsNewCommitsFromRemote(t *testing.T) {
	source := newTestSeedRepository(t)
	dest := newTestSeedRepository(t)

	seedPack, err := source.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack (seed): %v", err)
	}
	stageAndCommit(t, source, "pull-cli-node-1", "First", "alice", "first commit")
	pack, err := source.BuildPushPack(context.Background(), "main", seedPack.TargetCommit)
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("knownCommit"); got != seedPack.TargetCommit {
			t.Fatalf("knownCommit = %q, want %q", got, seedPack.TargetCommit)
		}
		envelope := buildTestPullEnvelope(t, pack.TargetCommit, [][]byte{pack.PackData})
		writeTestZstdPullResponse(t, w, envelope)
	}))
	defer server.Close()

	if err := dest.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPullCommand(func() (*repository.Repository, error) { return dest, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute pull: %v", err)
	}

	var result pullResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if !result.Pulled || result.Branch != "main" || result.CommitsInstalled != 1 || result.HeadCommit != pack.TargetCommit {
		t.Fatalf("result = %#v", result)
	}
}

func TestPullUpToDateReportsCleanly(t *testing.T) {
	repo := newTestSeedRepository(t)

	local, err := repo.BuildPushPack(context.Background(), "main", "")
	if err != nil {
		t.Fatalf("BuildPushPack: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Spool-Head-Commit", local.TargetCommit)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPullCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute pull: %v", err)
	}

	var result pullResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.Pulled || !result.UpToDate {
		t.Fatalf("result = %#v", result)
	}
}

func TestPullDivergedReportsRemoteHead(t *testing.T) {
	repo := newTestSeedRepository(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":       "conflict",
			"message":     "local branch diverged from rack",
			"currentHead": "some-other-head",
		})
	}))
	defer server.Close()
	if err := repo.SetRemote(repository.RemoteConfig{Endpoint: server.URL, RepoID: "acme", AuthMode: repository.RemoteAuthModeBearer}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}
	t.Setenv("SPOOL_RACK_TOKEN", "test-token")

	var output bytes.Buffer
	command := NewPullCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute pull: %v", err)
	}

	var result pullResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	if result.Pulled || !result.Diverged || result.ActualHead != "some-other-head" {
		t.Fatalf("result = %#v", result)
	}
}

func TestPullWithoutConfiguredRemoteFails(t *testing.T) {
	repo := newTestSeedRepository(t)

	var output bytes.Buffer
	command := NewPullCommand(func() (*repository.Repository, error) { return repo, nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != repository.ErrRemoteNotConfigured {
		t.Fatalf("err = %v, want ErrRemoteNotConfigured", err)
	}
}
