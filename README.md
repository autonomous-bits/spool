# Spool

Spool is a local, content-addressed version-control system for graph data. It keeps immutable
snapshots of nodes and edges, lets you stage and commit graph mutations, and provides local
branching, history, and snapshot comparison through the `spl` command-line interface.

It is designed for machine integration: successful commands emit JSON to standard output, while
failures are structured JSON logs on standard error.

## Install

Spool currently builds from source and requires Go **1.26.1** or later:

```sh
go install github.com/autonomous-bits/spool/cmd/spl@latest
```

To build the checked-out workspace instead:

```sh
go build -o dist/spl ./cmd/spl
```

## Quick start

Initialize a graph repository in your workspace:

```sh
spl init
```

This creates a `.spl` state directory with a default `main` branch. From a subdirectory, `spl`
locates the nearest parent `.spl` directory; when initializing, it uses the directory containing
`go.work`, or the current directory if none is found.

Stage a JSON mutation batch and commit it:

```sh
spl add --branch main --batch mutations.json
spl status --branch main
spl commit --branch main --author alice --message "Add graph data"
```

Create and use a branch:

```sh
spl branch create feature --from-branch main
spl switch feature
spl branch list
```

Query and compare graph snapshots:

```sh
spl resolve --branch main --node 11111111-1111-4111-8111-111111111111
spl diff --base-branch main --target-branch feature
spl history --branch main --entity-id 11111111-1111-4111-8111-111111111111
spl branches-containing --entity-id 11111111-1111-4111-8111-111111111111
```

Run `spl <command> --help` for all commands, flags, response-budget controls, and examples.

## Learn more

- [`CONTRIBUTING.md`](CONTRIBUTING.md) explains how to build, test, and contribute to Spool.
- [`docs/architecture.md`](docs/architecture.md) describes the high-level system architecture.
