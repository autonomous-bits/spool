package graphcontract_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/graphcontract"
)

const commitFixtureFormatVersion = 1

// commitFixture is the versioned, Rack-consumable conformance format for
// canonical Commit encoding, covering linear (single-parent) and merge
// (multi-parent) commits plus invalid commits and non-canonical rejection.
type commitFixture struct {
	FormatVersion    int          `json:"format_version"`
	Candidate        string       `json:"candidate"`
	Commit           *commitValue `json:"commit"`
	CanonicalCBORHex string       `json:"canonical_cbor_hex"`
	ObjectID         string       `json:"object_id"`
	Error            string       `json:"error"`
}

type commitValue struct {
	Snapshot string    `json:"snapshot"`
	Parents  []string  `json:"parents"`
	Message  string    `json:"message"`
	Author   string    `json:"author"`
	Time     time.Time `json:"time"`
}

func loadCommitFixtures(t *testing.T) []commitFixture {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "commits", "v1", "*.json"))
	if err != nil {
		t.Fatalf("list commit fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no commit conformance fixtures")
	}
	fixtures := make([]commitFixture, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read commit fixture %s: %v", path, err)
		}
		var fixture commitFixture
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatalf("decode commit fixture %s: %v", path, err)
		}
		if fixture.FormatVersion != commitFixtureFormatVersion {
			t.Fatalf("%s: format version = %d, want %d", path, fixture.FormatVersion, commitFixtureFormatVersion)
		}
		fixtures = append(fixtures, fixture)
	}
	return fixtures
}

// TestCommitConformanceFixtures proves canonical Commit encoding, decoding,
// and content-derived ObjectID computation are stable for linear and merge
// commits, and that invalid commits and non-canonical CBOR are rejected
// with stable errors.
func TestCommitConformanceFixtures(t *testing.T) {
	for _, fixture := range loadCommitFixtures(t) {
		fixture := fixture
		t.Run(fixture.Candidate, func(t *testing.T) {
			switch {
			case fixture.Commit != nil && fixture.Error == "":
				assertValidCommitFixture(t, fixture)
			case fixture.Commit != nil && fixture.Error != "":
				assertInvalidCommitFixture(t, fixture)
			case fixture.Commit == nil && fixture.CanonicalCBORHex != "" && fixture.Error != "":
				assertNonCanonicalCommitFixture(t, fixture)
			default:
				t.Fatalf("fixture has neither a valid commit nor a recognized error case")
			}
		})
	}
}

func commitParents(value *commitValue) []graphcontract.ObjectID {
	if len(value.Parents) == 0 {
		return nil
	}
	parents := make([]graphcontract.ObjectID, len(value.Parents))
	for i, parent := range value.Parents {
		parents[i] = graphcontract.ObjectID(parent)
	}
	return parents
}

func assertValidCommitFixture(t *testing.T, fixture commitFixture) {
	t.Helper()
	value := fixture.Commit
	commit, err := graphcontract.NewCommit(graphcontract.ObjectID(value.Snapshot), commitParents(value), value.Author, value.Message, value.Time)
	if err != nil {
		t.Fatalf("NewCommit: %v", err)
	}
	encoded, err := graphcontract.MarshalCommit(commit)
	if err != nil {
		t.Fatalf("MarshalCommit: %v", err)
	}
	if got := hex.EncodeToString(encoded); got != fixture.CanonicalCBORHex {
		t.Fatalf("commit canonical CBOR = %s, want %s", got, fixture.CanonicalCBORHex)
	}
	wantID := graphcontract.ObjectID(fixture.ObjectID)
	if got := graphcontract.ObjectIDForEncoded("commit", encoded); got != wantID {
		t.Fatalf("commit object id = %s, want %s", got, wantID)
	}
	decoded, err := graphcontract.UnmarshalCommit(encoded)
	if err != nil {
		t.Fatalf("UnmarshalCommit: %v", err)
	}
	if !decoded.Equal(commit) {
		t.Fatalf("decoded commit = %#v, want semantic equality with %#v", decoded, commit)
	}
}

func assertInvalidCommitFixture(t *testing.T, fixture commitFixture) {
	t.Helper()
	value := fixture.Commit
	if _, err := graphcontract.NewCommit(graphcontract.ObjectID(value.Snapshot), commitParents(value), value.Author, value.Message, value.Time); !errors.Is(err, graphcontract.ErrInvalidCommit) {
		t.Fatalf("NewCommit error = %v, want ErrInvalidCommit", err)
	}
}

func assertNonCanonicalCommitFixture(t *testing.T, fixture commitFixture) {
	t.Helper()
	data, err := hex.DecodeString(fixture.CanonicalCBORHex)
	if err != nil {
		t.Fatalf("decode fixture hex: %v", err)
	}
	if _, err := graphcontract.UnmarshalCommit(data); !errors.Is(err, graphcontract.ErrInvalidCanonicalCBOR) {
		t.Fatalf("UnmarshalCommit error = %v, want ErrInvalidCanonicalCBOR", err)
	}
}
