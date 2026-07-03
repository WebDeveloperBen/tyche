# `tyche`

The unified tyche CLI. One binary for everything: project setup, server codec
generation, client generation, and running build/test commands against the
generated worktree.

```sh
go install github.com/webdeveloperben/tyche/cmd/tyche@latest
```

## Subcommands

| Command | Purpose |
| --- | --- |
| `tyche init` | Scaffold a `tyche.json` next to `go.mod`. Prompts for the client module; flags skip the prompts. |
| `tyche config show` | Print the resolved config (path, every value, source). `--json` for machine-readable. |
| `tyche generate` | Generate server route manifests and codecs over the matching packages. |
| `tyche client` | Generate the typed Go client from the configured OpenAPI spec. |
| `tyche build` | Generate, then `go build` a package in a temporary worktree. |
| `tyche run` | Generate, then `go run` a package in a temporary worktree. |
| `tyche test` | Generate, then `go test` packages in a temporary worktree. |
| `tyche clean` | Remove generated route codec files from the working tree. |

## Global flags

| Flag | Effect |
| --- | --- |
| `--config <path>` | Load this `tyche.json` directly; bypass discovery. |
| `--quiet` | Suppress the "using config ..." info line. |
| `TYCHE_CONFIG` (env) | Same as `--config` when the flag is not set. |

## Config discovery

Without `--config` or `TYCHE_CONFIG`, the CLI walks up from cwd looking for
`tyche.json` (or `tyche.config.json`), stopping at the first `go.mod`. If no
file is found, the CLI falls back to flag-only behaviour — today's default.

## Example `tyche.json`

```json
{
  "version": 1,
  "spec": "./api/openapi.json",
  "client": {
    "out": "./client",
    "module": "github.com/acme/api/client",
    "type_naming": "structural"
  },
  "server": {
    "patterns": ["./..."],
    "ignore": ["./tmp", "./bin", "./.git"]
  }
}
```

| Field | Notes |
| --- | --- |
| `version` | Required. Must be `1`. |
| `spec` | OpenAPI document for `tyche client`. |
| `client.out` | Output directory for the generated client. |
| `client.module` | Go module path for the generated client. Required if `client` is set. |
| `client.type_naming` | `structural` (default) or `operation-scoped`. |
| `server.patterns` | Package patterns for `tyche generate`. |
| `server.ignore` | Paths excluded from generated worktrees (used by `tyche build`/`run`/`test`). |

Unknown top-level and nested fields are rejected. Validation runs at load time
so misconfigurations fail fast before any generation work.
