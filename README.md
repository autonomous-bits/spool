# Spool

Spool stores solution context as **human-diffable graph files in a git remote**. Git is the
durable source of truth. A bound code repository writes `.spool/context.toml`; context-management
commands rebuild a disposable local SQLite projection from that checkout and never commit it.

The public `spl` CLI and MCP server are **bound context-git tools**. Mutations open a short-lived
`spool/mcp/<stamp>-<nonce>` branch and a pull request into the protected branch. They never push
the protected branch except during `spl context init` onboarding.

Successful commands emit JSON to standard output. Failures are structured JSON logs on standard
error.

## Install

### Prebuilt binaries

Download the latest prebuilt archive for your platform from [GitHub Releases](https://github.com/autonomous-bits/spool/releases):

| Platform | Architecture | Archive |
| :--- | :--- | :--- |
| **macOS** | Apple Silicon (`arm64`) | `spool_<version>_darwin_arm64.tar.gz` |
| | Intel (`amd64`) | `spool_<version>_darwin_amd64.tar.gz` |
| **Linux** | ARM64 (`arm64`) | `spool_<version>_linux_arm64.tar.gz` |
| | x86-64 (`amd64`) | `spool_<version>_linux_amd64.tar.gz` |
| **Windows** | x86-64 (`amd64`) | `spool_<version>_windows_amd64.zip` |

Extract the archive and place the `spl` binary (or `spl.exe` on Windows) into a directory on your `PATH`. You can verify downloads with the accompanying `checksums.txt` file.

### Package managers (Upcoming)

Distribution through package managers is planned for upcoming releases:

- **Homebrew** (macOS / Linux): *Coming soon*
- **WinGet** (Windows): *Coming soon*

### Go install

If you have Go **1.26.1** or later installed:

```sh
go install github.com/autonomous-bits/spool/cmd/spl@latest
```

### Build from source

To build from a local clone of the repository:

```sh
git clone https://github.com/autonomous-bits/spool.git
cd spool
go build -o dist/spl ./cmd/spl
# or: make build
```

## Quick start

Bind this code repository to a solution context remote:

```sh
spl context init --remote https://github.com/org/solution-context.git
```

`context init` writes `.spool/context.toml` (`solution_id`, `remote`, `protected_branch`) and seeds
the remote layout when it is empty. Context-management commands refuse to run until that bind file
exists.

Query the bound checkout:

```sh
spl resolve --node idea-1
spl search --query incident
spl filter --label Task --property-text status=open
spl query-context --label Task --direction both
spl search-expand --query incident --direction out --edge-type RELATES_TO
spl graph
spl validate
```

Use stock git on the context remote for history and diff. Spool does not wrap `git log` or `git diff`.

Write routine node and edge mutations (one JSON batch → short-lived branch + PR):

```sh
spl mutate --operations mutations.json --message "Record requirement"
```

`mutate` requires `.spool/context.toml` and refuses unbound workspaces. Identical or empty
effective diffs do not open a PR. Use `schema migrate` when changing `schema.toml`.

## Context bind, export, and migrate-once

```sh
spl context init --remote https://github.com/org/solution-context.git
spl context export
spl context migrate-once
```

`context export` and `context migrate-once` read leftover private `.spl` graph state internally and
write human-diffable nodes/edges through a short-lived branch and pull request. They are not Spool
VCS commands. `migrate-once` skips the write when titles are already present.

The `context` namespace is **only** init/export/migrate-once. Graph queries use `query-context`.

## Mutate, schema, assets, merge, and prune

Write routine node and edge mutations as one JSON batch. Bound-only; opens a short-lived branch + PR:

```sh
spl mutate --operations mutations.json --author alice --message "Record requirement"
```

Author a schema in TOML and apply conforming graph mutations:

```toml
# people.toml
version = 2

[[node]]
label = "Person"
[[node.property]]
key = "name"
required = true
types = ["string"]
```

```sh
spl schema migrate --schema people.toml --batch people-mutations.json
spl validate
```

`schema migrate` validates the candidate graph against the target schema and opens a short-lived
branch + PR. An identical schema is a no-op (no empty PR).

Store a reference document as an Asset node (auto-commits via branch + PR):

```sh
spl asset add --file docs/architecture.md --title "Architecture notes"
spl asset read --node notes > architecture.md
```

Preview and apply a three-way **file-graph** merge (not a Rack/.spl lease):

```sh
spl merge preview --source feature --target main
spl merge apply --source feature --target main --transaction merge-42 --preview <preview-id> \
  --author alice --message "Merge feature"
```

Conflicted applies persist merge state in the context-git cache. Inspect, resolve, then finalize
or abort:

```sh
spl merge conflicts --transaction merge-42
spl merge resolve --transaction merge-42 --preview <preview-id> --selections selections.json
spl merge finalize --transaction merge-42
# or: spl merge abort --transaction merge-42
```

Remove temporary planning data labeled `Ephemeral` (graph cleanup, **not** pack/CAS GC):

```sh
spl prune --dry-run
spl prune --author alice --message "Prune transient plan"
```

`prune` requires a bound checkout, refuses unbound workspaces, and writes a short-lived branch + PR
when it deletes ephemeral nodes and cascading edges.

## MCP server (`spl mcp`)

Spool includes a native Model Context Protocol server built on the official Go SDK
([`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk)).
`spl mcp` exposes the **KEEP** tool set (20 tools) over stdio. The tool list matches `spl --help`.

### Client configuration

```json
{
  "mcpServers": {
    "spool": {
      "command": "spl",
      "args": ["mcp"]
    }
  }
}
```

```sh
spl mcp
```

## CLI command reference

The installed surface, including generated help, is documented in
[`.agents/skills/spool/references/cli-help.md`](.agents/skills/spool/references/cli-help.md).

| Command | Purpose |
| --- | --- |
| `context init` | Bind this code repo to a context git remote (`--remote` required) |
| `context export`, `context migrate-once` | Export leftover `.spl` into bound context git |
| `query-context` | Evidence-focused bounded graph context (replaces query-`context`) |
| `search`, `search-expand`, `filter`, `resolve`, `graph` | Bound checkout reads |
| `mutate` | Routine node/edge writes (one batch → branch+PR) |
| `schema migrate`, `validate` | Schema write (branch+PR) and validation |
| `asset add`, `asset read` | Reference assets on the bound checkout |
| `merge preview/apply/conflicts/resolve/finalize/abort` | File-graph merge |
| `prune` | Ephemeral node/edge cleanup (branch+PR; not pack GC) |
| `mcp` | MCP stdio server (KEEP tools only) |
| `version`, `help`, `completion` | Provenance, help, and shell completion |

### Removed

These commands and their MCP twins are **deleted** (no aliases):

`init`, `workspace *`, `remote *`, `push`, `pull`, `clone`, old `migrate`, `fsck`, `gc`, pack prune,
`cherry-pick`, `add`, `status`, `commit`, `branch`, `switch`, `history`, `diff`,
`branches-containing`.

Use stock git for history and diff on the context remote.

## Learn more

- [`CONTRIBUTING.md`](CONTRIBUTING.md) explains how to build, test, and contribute to Spool.
- [`docs/architecture.md`](docs/architecture.md) describes the high-level system architecture.
- [`CHANGELOG.md`](CHANGELOG.md) explains how release notes are generated.
