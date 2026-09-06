# Graph contract interoperability

`github.com/autonomous-bits/spool/graphcontract` is Spool's public,
dependency-light canonical graph contract. Rack must pin **`v2.1.0` or later**,
the first release containing the schema API and interoperability fixtures.

The package exports canonical `PropertyValue`, `Node`, `Edge`, and `Commit`
objects; `SchemaSnapshot`, `NodeLabelRule`, `EdgeTypeRule`, `PropertyRule`,
`Cardinality`, and `GlobalInvariant`; canonical CBOR helpers including
`MarshalSchemaSnapshot` and `UnmarshalSchemaSnapshot`; TOML schema decoding
through `DecodeSchemaTOML`, `DecodeSchemaTOMLReader`, and `ParseSchemaTOML`;
and `ValidateSchemaSnapshot`. Validation failures are stable
`*SchemaValidationError` values containing lexically ordered
`[]SchemaViolation` entries with `SchemaViolationCode` categories. Callers
must preserve typed values and use these objects directly; no JSON schema
translation or numeric coercion is supported.

Repository persistence, staging, projections, indexes, CLI commands, HTTP
transport, PostgreSQL metadata, and CAS are intentionally not exported by this
package. `internal/repository` remains the authority for atomically staging a
schema migration and graph mutation batch into one snapshot.

## Shared fixtures

Versioned Rack-consumable fixtures are under
`graphcontract/testdata/schema/v1/`. Every JSON candidate has:

```json
{
  "format_version": 1,
  "candidate": "graph|migration|merge",
  "schema": "schema.toml",
  "nodes": {"node-id": {"id": "node-id"}},
  "edges": {"edge-id": {"id": "edge-id", "source": "node-id", "target": "node-id"}},
  "violations": []
}
```

`schema.toml` is the canonical TOML source. A candidate with an empty
`violations` list must validate; otherwise the listed violations must exactly
match the normalized order, fields, and categories returned by
`ValidateSchemaSnapshot`. `invalid-schema.toml` must be rejected with both
`ErrInvalidSchemaTOML` and `ErrInvalidSchemaDefinition`. Version 1 covers
typed integer versus float values, nested lists/maps, natural keys, endpoint
labels, cardinality, global invariants, and valid and invalid graph,
migration, and merge candidates.
