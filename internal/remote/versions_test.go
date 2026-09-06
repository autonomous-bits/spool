package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
)

func TestNegotiateVersionsReportsMatchForCurrentVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]uint32{
			"packFormatVersion":         graphcontract.PackFormatVersion,
			"packIndexFormatVersion":    graphcontract.PackIndexFormatVersion,
			"packManifestFormatVersion": graphcontract.PackManifestFormatVersion,
		})
	}))
	defer server.Close()

	report, err := NegotiateVersions(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "")
	if err != nil {
		t.Fatalf("NegotiateVersions: %v", err)
	}
	for name, field := range map[string]FieldReport{
		"packFormatVersion":         report.PackFormatVersion,
		"packIndexFormatVersion":    report.PackIndexFormatVersion,
		"packManifestFormatVersion": report.PackManifestFormatVersion,
	} {
		if field.Status != FieldStatusMatch {
			t.Fatalf("%s status = %v, want match", name, field.Status)
		}
	}
}

func TestNegotiateVersionsReportsMismatchForDifferentVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]uint32{
			"packFormatVersion":         graphcontract.PackFormatVersion + 1,
			"packIndexFormatVersion":    graphcontract.PackIndexFormatVersion,
			"packManifestFormatVersion": graphcontract.PackManifestFormatVersion,
		})
	}))
	defer server.Close()

	report, err := NegotiateVersions(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "")
	if err != nil {
		t.Fatalf("NegotiateVersions: %v", err)
	}
	if report.PackFormatVersion.Status != FieldStatusMismatch {
		t.Fatalf("packFormatVersion status = %v, want mismatch", report.PackFormatVersion.Status)
	}
	if report.PackIndexFormatVersion.Status != FieldStatusMatch {
		t.Fatalf("packIndexFormatVersion status = %v, want match", report.PackIndexFormatVersion.Status)
	}
}

func TestNegotiateVersionsReportsUnknownWhenFieldOmitted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	report, err := NegotiateVersions(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "")
	if err != nil {
		t.Fatalf("NegotiateVersions: %v", err)
	}
	for name, field := range map[string]FieldReport{
		"packFormatVersion":         report.PackFormatVersion,
		"packIndexFormatVersion":    report.PackIndexFormatVersion,
		"packManifestFormatVersion": report.PackManifestFormatVersion,
	} {
		if field.Status != FieldStatusUnknown {
			t.Fatalf("%s status = %v, want unknown", name, field.Status)
		}
	}
}

func TestNegotiateVersionsReportsUnreachableWithoutCrashing(t *testing.T) {
	_, err := NegotiateVersions(context.Background(), NewClient(), Config{Endpoint: "http://127.0.0.1:1", RepoID: "acme", AuthMode: AuthModeBearer}, "")
	if err == nil {
		t.Fatal("NegotiateVersions error = nil, want ErrRemoteUnreachable")
	}
}

func TestNegotiateVersionsSendsBearerAuthorizationHeader(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer server.Close()

	if _, err := NegotiateVersions(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "secret-token"); err != nil {
		t.Fatalf("NegotiateVersions: %v", err)
	}
	if receivedAuth != "Bearer secret-token" {
		t.Fatalf("Authorization header = %q, want Bearer secret-token", receivedAuth)
	}
}

func TestNegotiateVersionsSendsAPIKeyHeader(t *testing.T) {
	var receivedKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-Api-Key")
		_ = json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer server.Close()

	if _, err := NegotiateVersions(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeAPIKey}, "secret-key"); err != nil {
		t.Fatalf("NegotiateVersions: %v", err)
	}
	if receivedKey != "secret-key" {
		t.Fatalf("X-Api-Key header = %q, want secret-key", receivedKey)
	}
}
