package graphcontract_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
)

const invalidPackFixtureFormatVersion = 1

// invalidPackFixture is the versioned, Rack-consumable conformance format
// for pack streams that must be rejected as corrupt or truncated before
// any object inside them is trusted.
type invalidPackFixture struct {
	FormatVersion int    `json:"format_version"`
	Name          string `json:"name"`
	PackBase64    string `json:"pack_base64"`
	FailureStage  string `json:"failure_stage"`
	Error         string `json:"error"`
}

// TestInvalidPackConformanceFixtures proves malformed pack streams -
// starting with a bad magic header and a header truncated below
// PackHeaderSize - are rejected with the stable ErrPackCorrupt error before
// any packed object is read.
func TestInvalidPackConformanceFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "pack", "v1", "invalid-*.json"))
	if err != nil {
		t.Fatalf("list invalid pack fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no invalid pack conformance fixtures")
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read invalid pack fixture: %v", err)
			}
			var fixture invalidPackFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("decode invalid pack fixture: %v", err)
			}
			if fixture.FormatVersion != invalidPackFixtureFormatVersion {
				t.Fatalf("format version = %d, want %d", fixture.FormatVersion, invalidPackFixtureFormatVersion)
			}
			packBytes, err := base64.StdEncoding.DecodeString(fixture.PackBase64)
			if err != nil {
				t.Fatalf("decode pack bytes: %v", err)
			}
			switch fixture.FailureStage {
			case "read_header":
				if _, err := graphcontract.ReadPackHeader(bytes.NewReader(packBytes)); err == nil {
					t.Fatal("read pack header: want error")
				}
			case "validate_header":
				header, err := graphcontract.ReadPackHeader(bytes.NewReader(packBytes))
				if err != nil {
					t.Fatalf("read pack header: %v", err)
				}
				if err := graphcontract.ValidatePackHeader(header); err == nil || !errors.Is(err, graphcontract.ErrPackCorrupt) {
					t.Fatalf("validate pack header error = %v, want ErrPackCorrupt", err)
				}
			default:
				t.Fatalf("unknown failure_stage %q", fixture.FailureStage)
			}
		})
	}
}
