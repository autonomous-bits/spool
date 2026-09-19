package graphcontract

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEncodeSchemaTOMLRoundTrip(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("testdata", "schema", "v1", "schema.toml"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	schema, err := DecodeSchemaTOML(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	encoded, err := EncodeSchemaTOML(schema)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	again, err := DecodeSchemaTOML(encoded)
	if err != nil {
		t.Fatalf("redecode: %v\n%s", err, encoded)
	}
	if !reflect.DeepEqual(schema, again) {
		t.Fatalf("round trip mismatch\nencoded:\n%s", encoded)
	}
}

func TestEncodeBuiltinSchemaTOML(t *testing.T) {
	t.Parallel()
	encoded, err := EncodeSchemaTOML(BuiltinSchemaSnapshot())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if !bytes.Contains(encoded, []byte("version = 1")) || !bytes.Contains(encoded, []byte("permissive = true")) {
		t.Fatalf("encoded builtin = %s", encoded)
	}
	decoded, err := DecodeSchemaTOML(encoded)
	if err != nil {
		t.Fatalf("decode builtin: %v", err)
	}
	if !reflect.DeepEqual(decoded, BuiltinSchemaSnapshot()) {
		t.Fatalf("decoded = %#v", decoded)
	}
}
