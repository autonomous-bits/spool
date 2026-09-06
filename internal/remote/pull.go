package remote

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/klauspost/compress/zstd"
	"lukechampine.com/blake3"
)

// pullTimeout bounds a single pull HTTP call. Pull responses can carry many
// packs, so this is considerably more generous than healthzTimeout.
const pullTimeout = 60 * time.Second

// Rack's v2 pull envelope framing (mirrored from spool-rack's
// internal/server/sync/pull.go): a 16-byte header (4-byte magic, 4-byte
// big-endian format version, 8-byte big-endian manifest length) followed by
// a canonical CBOR PullManifestV2 and then every declared pack's raw bytes,
// concatenated in manifest order.
const (
	pullEnvelopeMagic      = "SRPL"
	pullEnvelopeFormatV2   = uint32(2)
	pullEnvelopeHeaderSize = 16
)

// ErrInvalidPullEnvelope indicates Rack's pull response body was malformed,
// non-canonically encoded, or internally inconsistent (a declared pack
// length or hash that does not match the transmitted bytes).
var ErrInvalidPullEnvelope = errors.New("rack pull response is invalid")

// ErrPullBranchNotFound reports that Rack has no such branch for the
// configured repository.
var ErrPullBranchNotFound = errors.New("rack remote has no such branch")

// ErrPullRejected reports that Rack rejected a pull for a reason other than
// divergence or a missing branch.
var ErrPullRejected = errors.New("rack rejected the pull")

// pullCanonicalCBOR and pullStrictCBOR mirror the canonical-encode and
// strict-decode CBOR modes Rack's sync package uses for pull envelope
// framing, so this client accepts exactly the same well-formed, canonical
// encodings Rack is capable of producing and rejects anything else.
var (
	pullCanonicalCBOR, _ = cbor.CanonicalEncOptions().EncMode()
	pullStrictCBOR, _    = cbor.DecOptions{
		DupMapKey:         cbor.DupMapKeyEnforcedAPF,
		IndefLength:       cbor.IndefLengthForbidden,
		TagsMd:            cbor.TagsForbidden,
		ExtraReturnErrors: cbor.ExtraDecErrorUnknownField,
	}.DecMode()
)

// pullPackManifestV2 describes one raw pack within a pull envelope,
// mirroring Rack's sync.PullPackManifestV2.
type pullPackManifestV2 struct {
	Hash   string `cbor:"1,keyasint"`
	Format uint32 `cbor:"2,keyasint"`
	Length uint64 `cbor:"3,keyasint"`
}

// pullManifestV2 mirrors Rack's sync.PullManifestV2: the canonical manifest
// at the start of every version 2 pull envelope.
type pullManifestV2 struct {
	Version uint32               `cbor:"1,keyasint"`
	Head    string               `cbor:"2,keyasint"`
	Packs   []pullPackManifestV2 `cbor:"3,keyasint"`
}

// PullResult is the outcome of a successful pull HTTP call. When UpToDate is
// true, Packs is empty and HeadCommit is the branch's current wire commit
// ID. Otherwise Packs holds every hash-verified, canonically-framed v2 pack
// Rack sent, oldest-to-newest, and HeadCommit is the wire commit ID the
// caller advances to once every pack is installed.
type PullResult struct {
	UpToDate   bool
	HeadCommit string
	Packs      [][]byte
}

// DivergedError reports that the caller's supplied known commit is not an
// ancestor of Rack's current branch head. ActualHead is Rack's current wire
// head commit ID for the branch.
type DivergedError struct {
	ActualHead string
	Guidance   string
}

func (e *DivergedError) Error() string {
	if e.Guidance != "" {
		return e.Guidance
	}
	return fmt.Sprintf("pull diverged: remote head is %s", e.ActualHead)
}

// pullErrorEnvelope mirrors Rack's JSON error body shape:
// {"error": "...", "message": "...", "currentHead": "..."}.
type pullErrorEnvelope struct {
	Error       string `json:"error"`
	Message     string `json:"message"`
	CurrentHead string `json:"currentHead,omitempty"`
}

// pull calls GET {endpoint}/api/v1/repos/{repoID}/pull?branch=...&knownCommit=...
// and returns the decoded, hash-verified pull response. knownCommit may be
// empty to request the branch's entire history from scratch.
func (c *Client) pull(ctx context.Context, endpoint, repoID string, authMode AuthMode, credential string, branch, knownCommit string) (PullResult, error) {
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: pullTimeout}
	}

	query := url.Values{}
	query.Set("branch", branch)
	if knownCommit != "" {
		query.Set("knownCommit", knownCommit)
	}
	target := strings.TrimRight(endpoint, "/") + "/api/v1/repos/" + repoID + "/pull?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return PullResult{}, fmt.Errorf("build pull request: %w", err)
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
		return PullResult{}, fmt.Errorf("%w: %s", ErrRemoteUnreachable, Redact(err.Error(), credential))
	}
	defer func() { _ = response.Body.Close() }()

	switch response.StatusCode {
	case http.StatusNoContent:
		return PullResult{UpToDate: true, HeadCommit: response.Header.Get("X-Spool-Head-Commit")}, nil
	case http.StatusOK:
		return decodePullResponse(response)
	case http.StatusConflict:
		var envelope pullErrorEnvelope
		_ = json.NewDecoder(response.Body).Decode(&envelope)
		return PullResult{}, &DivergedError{ActualHead: envelope.CurrentHead, Guidance: envelope.Message}
	case http.StatusNotFound:
		return PullResult{}, ErrPullBranchNotFound
	default:
		var envelope pullErrorEnvelope
		_ = json.NewDecoder(response.Body).Decode(&envelope)
		message := envelope.Message
		if message == "" {
			message = fmt.Sprintf("pull returned status %d", response.StatusCode)
		}
		return PullResult{}, fmt.Errorf("%w: %s", ErrPullRejected, Redact(message, credential))
	}
}

// decodePullResponse reads and decompresses a 200 OK pull response body and
// splits it into a hash-verified, canonically-framed set of raw v2 packs.
func decodePullResponse(response *http.Response) (PullResult, error) {
	var body io.Reader = response.Body
	if response.Header.Get("Content-Encoding") == "zstd" {
		decoder, err := zstd.NewReader(response.Body)
		if err != nil {
			return PullResult{}, fmt.Errorf("%w: create zstd reader: %v", ErrInvalidPullEnvelope, err)
		}
		defer decoder.Close()
		body = decoder
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return PullResult{}, fmt.Errorf("%w: read pull response body: %v", ErrInvalidPullEnvelope, err)
	}

	head, packs, err := decodePullEnvelope(data)
	if err != nil {
		return PullResult{}, err
	}
	return PullResult{HeadCommit: head, Packs: packs}, nil
}

// decodePullEnvelope verifies data is a well-formed, canonically encoded v2
// pull envelope and splits it back into its individually bounded,
// hash-verified pack byte slices, in manifest order.
func decodePullEnvelope(data []byte) (head string, packs [][]byte, err error) {
	if len(data) < pullEnvelopeHeaderSize || string(data[:4]) != pullEnvelopeMagic {
		return "", nil, fmt.Errorf("%w: missing envelope header", ErrInvalidPullEnvelope)
	}
	if version := binary.BigEndian.Uint32(data[4:8]); version != pullEnvelopeFormatV2 {
		return "", nil, fmt.Errorf("%w: unsupported envelope version %d", ErrInvalidPullEnvelope, version)
	}
	manifestLength := binary.BigEndian.Uint64(data[8:pullEnvelopeHeaderSize])
	if manifestLength > uint64(len(data)-pullEnvelopeHeaderSize) {
		return "", nil, fmt.Errorf("%w: manifest length exceeds envelope", ErrInvalidPullEnvelope)
	}
	manifestEnd := pullEnvelopeHeaderSize + int(manifestLength)
	manifest, err := unmarshalPullManifestV2(data[pullEnvelopeHeaderSize:manifestEnd])
	if err != nil {
		return "", nil, err
	}
	if len(manifest.Packs) == 0 {
		return "", nil, fmt.Errorf("%w: manifest declares no packs", ErrInvalidPullEnvelope)
	}

	packs = make([][]byte, len(manifest.Packs))
	offset := manifestEnd
	for i, pack := range manifest.Packs {
		if pack.Length > uint64(len(data)-offset) {
			return "", nil, fmt.Errorf("%w: pack %d length exceeds envelope", ErrInvalidPullEnvelope, i)
		}
		end := offset + int(pack.Length)
		packData := data[offset:end]
		if pullContentID(packData) != pack.Hash {
			return "", nil, fmt.Errorf("%w: pack %d hash mismatch", ErrInvalidPullEnvelope, i)
		}
		packs[i] = packData
		offset = end
	}
	if offset != len(data) {
		return "", nil, fmt.Errorf("%w: trailing bytes after packs", ErrInvalidPullEnvelope)
	}
	return manifest.Head, packs, nil
}

// unmarshalPullManifestV2 verifies that data is a canonical, self-consistent
// v2 pull manifest.
func unmarshalPullManifestV2(data []byte) (pullManifestV2, error) {
	var manifest pullManifestV2
	if err := pullStrictCBOR.Unmarshal(data, &manifest); err != nil {
		return pullManifestV2{}, fmt.Errorf("%w: decode manifest: %v", ErrInvalidPullEnvelope, err)
	}
	if manifest.Version != pullEnvelopeFormatV2 {
		return pullManifestV2{}, fmt.Errorf("%w: unsupported manifest version %d", ErrInvalidPullEnvelope, manifest.Version)
	}
	canonical, err := pullCanonicalCBOR.Marshal(manifest)
	if err != nil {
		return pullManifestV2{}, fmt.Errorf("%w: re-encode manifest: %v", ErrInvalidPullEnvelope, err)
	}
	if !bytes.Equal(data, canonical) {
		return pullManifestV2{}, fmt.Errorf("%w: manifest is not canonically encoded", ErrInvalidPullEnvelope)
	}
	return manifest, nil
}

// pullContentID returns the lowercase BLAKE3-256 content ID Rack's pull
// protocol uses for pack identity, matching pushContentID's scheme.
func pullContentID(data []byte) string {
	sum := blake3.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Pull fetches new commits for branch from cfg.Endpoint/cfg.RepoID, starting
// after knownCommit (empty to fetch the branch's entire history).
// credential, when non-empty, is attached per cfg.AuthMode. A diverged known
// commit is returned as a *DivergedError (use errors.As); a missing branch
// is wrapped in ErrPullBranchNotFound; any other rejection is wrapped in
// ErrPullRejected; an unreachable or malformed-response remote is wrapped in
// ErrRemoteUnreachable or ErrInvalidPullEnvelope.
func Pull(ctx context.Context, client *Client, cfg Config, credential string, branch, knownCommit string) (PullResult, error) {
	if client == nil {
		client = NewClient()
	}
	return client.pull(ctx, cfg.Endpoint, cfg.RepoID, cfg.AuthMode, credential, branch, knownCommit)
}
