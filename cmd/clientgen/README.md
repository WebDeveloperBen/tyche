# `clientgen`

`clientgen` generates a self-contained, dependency-free typed Go client from a
tyche-produced OpenAPI JSON document (the spec your tyche server emits).

It is the same generator exposed by `servergen client`, shipped as a standalone
binary so it can be installed and run on its own:

```sh
go install github.com/webdeveloperben/tyche/cmd/clientgen@latest
```

## Usage

```sh
go run ./cmd/clientgen \
  --spec ./openapi.json \
  --out ./client \
  --module github.com/you/app/client \
  --type-naming structural
```

## Flags

| Flag            | Required | Description                                                             |
| --------------- | -------- | ----------------------------------------------------------------------- |
| `--spec`        | yes      | Path to the OpenAPI JSON document                                       |
| `--out`         | yes      | Output directory for the generated client module                        |
| `--module`      | yes      | Go module path for the generated client                                 |
| `--package`     | no       | Package name for generated files (default: derived from module)         |
| `--go`          | no       | `go` directive for the generated `go.mod` (default: 1.22)               |
| `--client-name` | no       | Generated client type name (default: `Client`)                          |
| `--type-naming` | no       | `structural` or `operation-scoped` (default: `structural`)              |

## What it generates

A dependency-free Go module that imports only the standard library:

- its own `go.mod`
- request/response types (structurally deduplicated by default, or
  operation-scoped with `--type-naming operation-scoped`)
- one method per OpenAPI operation
- typed `application/problem+json` errors (surfaced as `*APIError`)
- typed Server-Sent Events streaming methods for `text/event-stream` operations

An existing `go.mod` in the output directory is left untouched so module-level
customizations (a `replace`, a bumped `go` directive) survive regeneration.
Stale generated `.go` files from a previous run are removed.
