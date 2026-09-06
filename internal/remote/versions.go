package remote

import (
	"context"

	"github.com/autonomous-bits/spool/graphcontract"
)

// FieldStatus reports how a single graphcontract version field compares
// against this build's constants.
type FieldStatus string

const (
	// FieldStatusMatch reports that the remote's reported version equals
	// this build's constant.
	FieldStatusMatch FieldStatus = "match"
	// FieldStatusMismatch reports that the remote's reported version
	// differs from this build's constant.
	FieldStatusMismatch FieldStatus = "mismatch"
	// FieldStatusUnknown reports that the remote's response omitted the
	// field, so no comparison could be made.
	FieldStatusUnknown FieldStatus = "unknown"
)

// FieldReport is the negotiated status of one graphcontract version field.
type FieldReport struct {
	Local  uint32      `json:"local"`
	Remote *uint32     `json:"remote,omitempty"`
	Status FieldStatus `json:"status"`
}

// VersionReport is the full graphcontract version negotiation result for one
// remote.
type VersionReport struct {
	PackFormatVersion         FieldReport `json:"packFormatVersion"`
	PackIndexFormatVersion    FieldReport `json:"packIndexFormatVersion"`
	PackManifestFormatVersion FieldReport `json:"packManifestFormatVersion"`
}

// NegotiateVersions probes cfg.Endpoint's /healthz and compares any reported
// graphcontract version fields against this build's constants. credential,
// when non-empty, is attached to the request per cfg.AuthMode. A remote
// response that omits a version field is reported explicitly as
// FieldStatusUnknown rather than causing an error; a remote that cannot be
// reached at all, or that returns a non-200/undecodable response, causes
// NegotiateVersions to return a wrapped ErrRemoteUnreachable instead of a
// VersionReport, which callers should handle explicitly (e.g. `spl remote
// show` reports this as an "unreachable" status rather than failing).
func NegotiateVersions(ctx context.Context, client *Client, cfg Config, credential string) (VersionReport, error) {
	if client == nil {
		client = NewClient()
	}
	health, err := client.fetchHealthz(ctx, cfg.Endpoint, cfg.AuthMode, credential)
	if err != nil {
		return VersionReport{}, err
	}
	// An older or not-yet-updated Rack server may omit the graphcontract
	// block entirely; treat that the same as every field within it being
	// omitted, rather than as an error.
	info := health.Graphcontract
	if info == nil {
		info = &healthzGraphcontractInfo{}
	}
	return VersionReport{
		PackFormatVersion:         fieldReport(graphcontract.PackFormatVersion, info.PackFormatVersion),
		PackIndexFormatVersion:    fieldReport(graphcontract.PackIndexFormatVersion, info.PackIndexFormatVersion),
		PackManifestFormatVersion: fieldReport(graphcontract.PackManifestFormatVersion, info.PackManifestFormatVersion),
	}, nil
}

func fieldReport(local uint32, remote *uint32) FieldReport {
	report := FieldReport{Local: local, Remote: remote, Status: FieldStatusUnknown}
	if remote != nil {
		report.Status = FieldStatusMismatch
		if *remote == local {
			report.Status = FieldStatusMatch
		}
	}
	return report
}
