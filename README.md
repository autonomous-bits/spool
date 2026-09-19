# Spool

Spool is the MCP/CLI tool for shared **solution context**. N code repositories
bind to **one** context git remote. **Git is the only durable source of truth.**
Spool validates mutations, writes human-diffable JSON, opens a short-lived
branch and pull request, and rebuilds a local query projection.

Successful commands emit JSON to standard output; failures are structured JSON
logs on standard error.

There is no Spool-specific remote protocol and no Rack dependency in the default
install: clone, pull requests, history, and credentials use **stock git**.

## Install

The default `spl` install does **not** depend on Rack or `spool-rack`. Do not
install a Rack service, configure a Rack remote, or use a Spool-specific token
for context sync.

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

That module path does not require Rack.

### Build from source

To build from a local clone of the repository:

```sh
git clone https://github.com/autonomous-bits/spool.git
cd spool
go build -o dist/spl ./cmd/spl
# or: make build
```

## Quick start

Bind each code repo to the shared solution context remote. Spool does not
auto-discover remotes from directory names, monorepo layout, leftover `.spl`
state, or Rack config.

```toml
# .spool/context.toml in every participating code repo
solution_id = "my-solution"
remote = "https://github.com/org/my-solution-context.git"
protected_branch = "main"
# repository_id = "github.com/org/svc-api"  # optional; namespaces node IDs
```

Or write that file with onboarding:

```sh
# In each code repo; same --remote and --solution-id, distinct --repository-id
spl context init --remote https://github.com/org/my-solution-context.git \
  --solution-id my-solution \
  --repository-id github.com/org/svc-api
```

`context init` creates `schema.toml`, `nodes/`, `edges/`, and `assets/` on an
empty remote and seeds a `CodeRepository` node from **this** bind. Repeat in
other code repos.

MCP writes fail closed until a bind exists. After that, **every** agent write
creates a short-lived branch and opens a pull request to `protected_branch`.
Do not push cleanly to the working or protected branch. Sync is stock
`git clone` / GitHub PR / history with **stock git credentials** only.

See [docs/context-bind.md](docs/context-bind.md) for the bind format and
multi-repo story.

## Write model

| Rule | Behavior |
| --- | --- |
| Source of truth | The bound context git remote. Local SQLite/FTS projections rebuild on every MCP/process start and are never committed. |
| Agent writes | One mutation batch → one git commit on a **short-lived branch** → **pull request** to `protected_branch`. |
| Overlaps | Humans resolve on the PR. No silent overwrite. |
| Auth | Stock git credentials only. Never a Spool-specific token. |
| Assets | Git LFS at or above **512 KiB** (configurable). Text, JSON, and TOML stay plain git (warn above ~1 MiB). |

## Migrating leftover `.spl` graphs

Existing `.spl` graphs move with a **one-shot** export — the documented escape
hatch, not a second store:

```sh
spl context export --branch main
# alias:
spl context migrate-once --branch main
```

That path is lossy, overwrite-at-own-risk, and **not sync**. It opens a
short-lived branch and PR. See [docs/context-git-migration.md](docs/context-git-migration.md).

## Unsupported for solution context

These are **deprecated / migration-only**. They are not a supported parallel
source of truth and must not be dual-run with the context git remote:

- Spool-as-VCS (`spl init`, local `.spl` packs as durable context)
- Rack sync (`spl remote`, `spl push`, `spl pull`, `spl clone`)
- `.spl` as durable SoT
- pack wire-compat with Rack
- detached `.spl` workspaces as the N-repo join

When `.spool/context.toml` is present, `spl init`, `spl workspace …`,
`spl remote`, `spl push`, `spl pull`, and `spl clone` refuse. MCP tools for
those paths always fail closed. Unbound leftover `.spl` state is only for
`spl context export`.

## MCP server (`spl mcp`)

Spool includes a native Model Context Protocol (MCP) server built on the official Go SDK ([`github.com/modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk)). It exposes Spool commands as structured MCP tools (`spl_*`) over standard I/O.

### Default vs Fallback Behavior for AI Agents

- **Default (MCP Tools)**: For AI agents, the MCP server is the **primary, recommended interface**. Bound writes land as a short-lived branch + PR.
- **Fallback (CLI)**: Use `spl` directly in scripts or CI without MCP.

### Configuration Examples

Run `spl mcp` from a code repo that contains `.spool/context.toml` (or after
`spl context init --remote`). Do not point `--state-dir` at `.spl` for solution
context.

#### Claude Desktop / Claude Code (`claude_desktop_config.json`)
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

#### VS Code / Cursor (`mcp.json`)
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

### Manual Execution

```sh
# Start the MCP server over standard I/O from a bound code repo
spl mcp
```

## CLI command reference

The complete installed surface, including generated help and every flag, is documented in
[`.agents/skills/spool/references/cli-help.md`](.agents/skills/spool/references/cli-help.md).
The command and flag inventory is:

| Command | Flags and positional arguments |
| --- | --- |
| `context init` | `--remote` (required), `--solution-id`, `--protected-branch`, `--repository-id`, `--author` |
| `context export` (`migrate-once`) | `--branch`, `--author`, `--message` |
| `add` | `--branch` (required), `--batch` (required) |
| `status` | `--branch` |
| `commit` | `--branch` (required), `--author`, `--message` |
| `branch create <name>` | exactly one of `--from-branch`, `--from-commit` |
| `branch list` | none |
| `branch delete <name>` | none; branch must be inactive and non-default |
| `switch <branch>` | positional branch |
| `schema migrate` | `--branch`, `--schema`, `--batch` (all required) |
| `validate` | `--branch` (required), `--commit` (reachable) |
| `resolve` | `--branch` (required), `--commit`, `--node`, `--max-rows`, `--max-response-bytes`, `--timeout`, `--max-depth`, `--max-visited` |
| `graph` | `--branch` (required) |
| `search` | `--branch`, `--query` (required), `--commit`, `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `filter` | `--branch` (required), `--commit`, repeatable `--label`/property predicates, `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `search-expand`, `context` | `--branch` (required), `--commit`, query or typed filters, `--direction`, repeatable `--edge-type`, `--seed-limit`, `--max-depth`, `--max-visited`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `history` | `--branch`, `--entity-id` (required), `--commit`, `--all-parents`, `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `branches-containing` | exactly one selector (`--entity-id`, `--snapshot-id`, `--natural-key`), `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `diff` | `--base-branch`, `--target-branch` (required), optional commits, repeatable `--node-id`/`--edge-id`, `--node-title-contains`, `--one-hop`, `--continuation`, `--max-rows`, `--max-response-bytes`, `--timeout` |
| `merge preview` | `--source`, `--target` (required) |
| `merge apply` | `--source`, `--target`, `--transaction`, `--preview` (required), `--author`, `--message` |
| `merge conflicts` | `--target`, `--transaction` (required) |
| `merge resolve` | `--target`, `--transaction`, `--preview`, `--selections` (required), `--overrides` |
| `merge finalize`, `merge abort` | `--target`, `--transaction` (required) |
| `fsck` | none; read-only integrity report |
| `gc` | `--dry-run`, `--repack`, `--grace-period` (default `336h`) |
| `prune` | `--branch` (required), `--dry-run`, `--force`, `--author`, `--message` |
| `cherry-pick` | `--commit` (required), `--target-branch` (required), `--dry-run`, `--author`, `--message` |
| `asset add` | `--branch` and `--file` (required), `--title`, `--id` |
| `asset read` | positional locator/node ID or `--locator`/`--node`, `--branch` |
| `mcp` | none; runs the official Model Context Protocol server over standard I/O |
| `version` | none |
| `completion` | shell subcommand: `bash`, `zsh`, `fish`, or `powershell` |
| `help [command path]` | optional command path |
| `init` | **deprecated / migration-only** — not solution-context onboarding |
| `workspace init <name>` | **deprecated / migration-only** |
| `workspace attach [path]` | **deprecated / migration-only** |
| `workspace migrate`, `migrate` | `--from` (required), `--to` (required); leftover `.spl` format upgrades only |
| `remote set/show/remove/branch` | **unsupported** for solution context (Rack sync sunset) |
| `clone`, `workspace clone` | **unsupported** for solution context; `git clone` the bind remote |
| `push`, `pull` | **unsupported** for solution context; agent writes are branch + PR |

The common query-budget flags are `--max-rows`, `--max-response-bytes`, and `--timeout`;
traversal commands additionally use `--max-depth` and `--max-visited` as listed in the full
reference. Every command also accepts `-h, --help` and the global `--state-dir`.

Run `spl <command> --help` for flags, response-budget controls, and examples.

## Learn more

- [`docs/context-bind.md`](docs/context-bind.md) documents the bind file and N-repos → one context remote.
- [`docs/context-git-migration.md`](docs/context-git-migration.md) documents one-shot `.spl` export (`spl context export`).
- [`CONTRIBUTING.md`](CONTRIBUTING.md) explains how to build, test, and contribute to Spool.
- [`docs/architecture.md`](docs/architecture.md) describes the high-level system architecture.
- [`CHANGELOG.md`](CHANGELOG.md) explains how release notes are generated.
