package assets

import (
	"bytes"
	"strings"
	"testing"
)

func TestIconAsset(t *testing.T) {
	if len(IconPNG) == 0 {
		t.Fatal("expected embedded IconPNG to be non-empty")
	}

	// Verify PNG magic header: 0x89 0x50 0x4E 0x47 0x0D 0x0A 0x1A 0x0A
	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if !bytes.HasPrefix(IconPNG, pngHeader) {
		t.Fatal("IconPNG does not have a valid PNG header")
	}

	prefix := "data:image/png;base64,"
	if !strings.HasPrefix(IconPNGDataURI, prefix) {
		t.Fatalf("expected IconPNGDataURI to start with %q", prefix)
	}

	if len(IconPNGDataURI) <= len(prefix) {
		t.Fatal("expected IconPNGDataURI to contain base64 payload")
	}
}
