package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/autonomous-bits/spool/graphcontract"
)

func TestNegotiateVersionsReportsMatchForCurrentVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "healthy",
			"graphcontract": map[string]uint32{
				"packFormatVersion":         graphcontract.PackFormatVersion,
				"packIndexFormatVersion":    graphcontract.PackIndexFormatVersion,
				"packManifestFormatVersion": graphcontract.PackManifestFormatVersion,
			},
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
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "healthy",
			"graphcontract": map[string]uint32{
				"packFormatVersion":         graphcontract.PackFormatVersion + 1,
				"packIndexFormatVersion":    graphcontract.PackIndexFormatVersion,
				"packManifestFormatVersion": graphcontract.PackManifestFormatVersion,
			},
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

func TestNegotiateVersionsSendsCorrelationIDHeader(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get(CorrelationIDHeader)
		_ = json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer server.Close()

	client := NewClient()
	if _, err := NegotiateVersions(context.Background(), client, Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, ""); err != nil {
		t.Fatalf("NegotiateVersions: %v", err)
	}
	if received == "" {
		t.Fatal("healthz request did not include a correlation ID header")
	}
	if received != client.CorrelationID {
		t.Fatalf("correlation ID header = %q, want client.CorrelationID = %q", received, client.CorrelationID)
	}
}

func TestNegotiateVersionsUnreachableWrapsRackErrorWithCorrelationID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":         "internal_error",
			"message":       "unexpected failure",
			"correlationId": "corr-500",
		})
	}))
	defer server.Close()

	_, err := NegotiateVersions(context.Background(), NewClient(), Config{Endpoint: server.URL, RepoID: "acme", AuthMode: AuthModeBearer}, "")
	if !errors.Is(err, ErrRemoteUnreachable) {
		t.Fatalf("err = %v, want ErrRemoteUnreachable", err)
	}
	var rackErr *RackError
	if !errors.As(err, &rackErr) {
		t.Fatalf("err = %v, want *RackError", err)
	}
	if rackErr.Code != "internal_error" || rackErr.CorrelationID != "corr-500" {
		t.Fatalf("rackErr = %#v", rackErr)
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

func TestNegotiateVersionsSendsTenantHeader(t *testing.T) {
	var receivedTenant string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTenant = r.Header.Get("X-Tenant-ID")
		_ = json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer server.Close()

	cfg := Config{
		Endpoint:    server.URL,
		TenantID:    "tenant-abc",
		WorkspaceID: "ws-xyz",
		AuthMode:    AuthModeBearer,
	}
	if _, err := NegotiateVersions(context.Background(), NewClient(), cfg, ""); err != nil {
		t.Fatalf("NegotiateVersions: %v", err)
	}
	if receivedTenant != "tenant-abc" {
		t.Fatalf("X-Tenant-ID header = %q, want tenant-abc", receivedTenant)
	}
}

