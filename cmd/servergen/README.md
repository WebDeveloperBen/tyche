# `servergen`

`servergen` is the CLI for typed `server.Register(...)` code generation and generated-worktree workflows.

## Commands

Generate code into the current working tree:

```sh
go run ./cmd/servergen generate ./...
```

Remove generated files from the current working tree:

```sh
go run ./cmd/servergen clean
```

Build from a temporary generated worktree without persisting generated files:

```sh
go run ./cmd/servergen build -o ./bin/api ./cmd/api
```

Run from a temporary generated worktree:

```sh
go run ./cmd/servergen run ./cmd/api
```

Test from a temporary generated worktree:

```sh
go run ./cmd/servergen test ./...
```

## What it generates

For each package containing `server.Register(...)` calls, `servergen generate` writes:

- `zz_server_routes_gen.go`

That file registers:

- generated route manifests
- generated request codecs
- generated response codecs

## Runtime model

Handwritten route declarations are still the source of truth.

Generated code does not replace handlers. It only replaces the transport-layer work around them:

- request binding
- validation
- response writing
