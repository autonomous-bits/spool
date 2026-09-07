# Graph contract interoperability

`github.com/autonomous-bits/spool/graphcontract` is Spool's public,
dependency-light canonical graph contract. Rack must pin **`v1.4.0` or
later**, the first release containing the schema API and interoperability
fixtures; a subsequent release adds the full object, commit, and invalid-pack
conformance fixture sets described below (see `CHANGELOG.md` for the exact
version). Before Rack upgrades its pinned Spool dependency, it executes these
shared contract fixtures in Rack CI, and Spool runs the same fixtures as
compatibility tests in Spool CI, so neither system owns a divergent
interpretation.

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

## Pack verification

`graphcontract` also exports the immutable pack container format and the
functions required to fully verify a pack without any filesystem or storage
coupling: `PackID`, `PackCompression` (`PackCompressionZstd`), `PackMetadata`,
`PackManifest`, `PackIndexEntry`, `PackHeader`, `PackIndexMetadata`,
`PackCorruptionError`, and `UnsupportedPackVersionError`, alongside
`ValidPackID`, `ReadPackHeader`/`ReadPackHeaderAt`, `ValidatePackHeader`,
`ValidatePackManifest`, `ValidatePackIndexMetadata`, `ValidatePackIndexEntry`,
and `ValidatePackEntries`.

Full packed-object integrity verification — CRC32 of the compressed bytes,
zstd decompression, decompressed-size confirmation, canonical CBOR envelope
decoding, and content-addressed object-hash confirmation — is exposed through
`DecompressPackedObject` (and its `DecodePackedObjectEnvelope` /
`ObjectIDForEncoded` building blocks). A caller holding a pack's header, its
index entries, and the corresponding compressed bytes can verify every
packed object without depending on `internal/repository`, which is what lets
Rack index native pack uploads directly against this package.

On-disk pack layout, the sidecar index file's binary encoding, pack
generation/reader lifecycle, and GC orchestration remain
`internal/repository` implementation details; only the pure pack container
format and its verification are part of this contract. Fixtures for the pack
format are under `graphcontract/testdata/pack/v1/`, including
`invalid-bad-magic.json` and `invalid-truncated-header.json`, which prove two
distinct failure stages before any packed object is read: a pack stream
truncated below `PackHeaderSize` fails in `ReadPackHeader` with a wrapped
`io.ReadFull` read error (e.g. `unexpected EOF`), while a pack stream with a
corrupted magic header decodes successfully but is rejected by
`ValidatePackHeader` with `ErrPackCorrupt`.

## Object fixtures

Versioned canonical-object fixtures are under
`graphcontract/testdata/objects/v1/`. Each fixture either carries a `node` or
`edge` `value` plus its expected `canonical_cbor_hex` and content-derived
`object_id` (computed via `ObjectIDForEncoded("node", ...)` or
`ObjectIDForEncoded("edge", ...)`), or an `error` describing why the value
must be rejected. Version 1 covers every `PropertyValue` kind (null, bool,
integer, float, string, list, map, including nested collections),
label deduplication and sorting, values with absent versus explicitly empty
collections, an unknown property kind (`ErrInvalidPropertyValue`), and
non-canonical CBOR for both `Node` and `Edge` (`ErrInvalidCanonicalCBOR`).

## Commit fixtures

Versioned commit fixtures are under `graphcontract/testdata/commits/v1/`.
Each fixture carries a `commit` (snapshot, parents, message, author, time)
plus its expected `canonical_cbor_hex` and `object_id`
(`ObjectIDForEncoded("commit", ...)`), or an `error` for commits that must
be rejected. Version 1 covers a root commit (no parents), a linear commit
(one parent), a merge commit (two parents, order preserved), invalid commits
missing a snapshot or carrying an empty parent ID (`ErrInvalidCommit`), and
non-canonical CBOR (`ErrInvalidCanonicalCBOR`).

## Fixture manifest

`graphcontract/testdata/MANIFEST.json` is the single, language-agnostic index
of every fixture set (schema, pack, objects, commits): its directory,
`format_version`, and a short description. A second repository's
conformance runner — such as Rack's — reads this manifest to discover and
iterate every fixture set without hardcoding per-language paths, and Spool's
own `TestFixtureManifestListsEveryFixtureSet` proves every listed directory
exists and stays non-empty.

## Snapshot roots

`graphcontract` also exports the canonical graph snapshot root: `Snapshot`,
the immutable, content-addressed root set for one graph version, referencing
the node, edge, incoming-adjacency, outgoing-adjacency, and schema tree
roots, plus their entity counts. `NewSnapshot` constructs a normalized
value, and `MarshalSnapshot`/`UnmarshalSnapshot` provide canonical CBOR
encoding with round-trip byte-identity verification (`ErrInvalidCanonicalCBOR`
on non-canonical input), matching the `Node`/`Edge`/`Commit` conventions.

A `Snapshot`'s content-derived `ObjectID` is computed the same way as every
other graph object: `ObjectIDForEncoded("graph-snapshot", encoded)` over its
canonical CBOR encoding. `Snapshot` has no nested collections to sort or
deduplicate, so `Normalize` is a defensive copy; `Equal` and `Clone` follow
the same semantics as the other canonical types.

Building, storing, and reconstructing the node/edge/adjacency/schema trees
that a `Snapshot` references remains `internal/repository` implementation
detail; only the canonical root-set record itself is part of this contract.

## Merge simulation

`graphcontract` also exports the deterministic three-way merge simulation
contract through `ThreeWayMerge`. Given base, source, and target node and edge
maps plus their schema roots, it merges against the common base by canonical
object ID and returns a `MergeResult` containing the merged node and edge maps,
the resolved schema root, a `Clean` flag, and stable `Changes` and
`Conflicts`.

`ThreeWayMerge` intentionally stops at structural merge simulation. It reports
structural conflicts in node and edge content plus schema-root conflicts, but
it does not resolve the chosen schema root to a `SchemaSnapshot` or run schema
validation itself. Callers that have repository context to do that work — such
as `internal/repository`, with branch, commit, and schema-snapshot resolution
available — validate afterward and append semantic conflicts and normalized
violations deterministically.

`MergeConflict.Category` is one of `"structural"`, `"schema"`, or
`"semantic"`, and `MergeConflict.Entity` is one of `"node"`, `"edge"`, or
`"schema"`. `SortMergeConflicts`, `MergeConflictID`, `MergeConflictPaths`, and
`SchemaViolationPaths` are public so callers that append their own semantic
conflicts after validation can reuse the exact same deterministic
ordering, identifiers, and graph paths as Spool itself, including Rack and
other downstream integrations.
