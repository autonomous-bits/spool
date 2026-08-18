# spl CLI

`cmd/spl` is the Cobra CLI module; it uses the root module through `go.mod`'s `replace`.

See the [architecture guide](../../docs/architecture.md) for the system-level
component boundaries, data model, and persistence design.

Commands:

- From the repository root: `go build ./cmd/spl`, `go test ./cmd/spl/...`

Conventions:

- Define subcommands in `commands/`; wire them in `root.go`.
- Encode successful results as JSON to stdout; return errors for `main` to log to stderr.
- Keep command validation in Cobra (`Args`, flags, `SilenceUsage`).
- Keep command-specific tests in a matching `commands/<command>_test.go` file. Do not place a
  command's tests in another command's test file; shared command behavior belongs in a clearly
  named shared test file only when it genuinely covers multiple commands.
