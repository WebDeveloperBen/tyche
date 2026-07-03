# Changelog

## Unreleased

### Breaking changes

- **Unified CLI.** The `servergen` and `clientgen` binaries are gone. Install
  the single `tyche` binary instead: `go install github.com/webdeveloperben/tyche/cmd/tyche@latest`.
  Subcommands: `init`, `config`, `generate`, `client`, `build`, `run`, `test`,
  `clean`, `version`, `completion`. `servergen client` → `tyche client`;
  `servergen generate` → `tyche generate`. The two underlying libraries
  (`servergen`, `clientgen`) remain importable for programmatic use; their old
  `cli` packages are deleted in favour of the new internal CLI.

- **CLI framework is Kong, not Cobra.** The user-facing CLI uses
  `github.com/alecthomas/kong` for struct-based command definitions. The
  `spf13/cobra` and `spf13/pflag` deps are gone from the main module.

- **Output format flag is `--format`.** The new global flag is `--format` with
  values `human|json|quiet`. It replaces the per-command `--json`/`--quiet`
  pairs on the old Cobra CLI; the new `config show` command still accepts
  `--json` as a shorthand for `--format=json` for back-compat.

- **Comparison benchmark moved to a sub-module.** `server/benchmarks_router_test.go`
  and the chi reference adapter in `server/adapter_spike_test.go` are gone
  from the main module and now live under `benchmarks/comparison/`, which
  has its own `go.mod` and pulls in chi/gin/huma/validator only when
  consumers run those benchmarks. Running the main module's tests no
  longer requires these third-party routers.

### Features

- **`tyche version`** prints the build identity (version, commit, build
  date, Go version, built-by) populated via `-ldflags`.

- **`tyche completion <shell>`** emits a shell completion script for
  bash, zsh, fish, or powershell.

- **Global `--format` flag.** Every command can be asked to render its
  result as `human` (default), `json`, or `quiet`. JSON output for
  `config show` is now a structured object rather than a string-encoded
  blob. Errors in JSON mode are emitted as RFC 9457 problem+json.

- **Structured CLI architecture.** The CLI is split into four internal
  packages: `internal/cli` (Kong-only), `internal/app` (use-case
  orchestrators that take plain Go values), `internal/output` (printer
  with human/json/quiet modes), and `internal/ui` (TTY detection and
  prompts). The servergen and clientgen libraries do not import any of
  these, so embedding them in another tool does not pull in Kong.

- **`tyche.json` config file** (sqlc-style). Place at the project root; the
  CLI discovers it by walking up from cwd to the first `go.mod`. Flags always
  override file values. Use `--config <path>` or `TYCHE_CONFIG=<path>` to
  point at a non-default file. Loader is `internal/config/`, stdlib
  `encoding/json` only — no new dependencies.
- **`tyche init`** scaffolds a `tyche.json` next to `go.mod`. Refuses to
  overwrite without `--force`. Prompts for the client module; `--yes` plus
  the right flags makes it non-interactive.
- **`tyche config show`** prints the resolved config (path, every value,
  source). `--json` for machine-readable output.
- **Multipart request support** in generated clients. Operations with
  `multipart/form-data` request bodies generate `form`, `file`, and `files`
  input fields. File inputs use the generated `client.File` type.
- **Strict 415 / 406 server-side.** JSON and multipart request bodies return
  415 for unsupported `Content-Type`; JSON and SSE responses return 406 when
  the request `Accept` header does not allow the produced media type.
- **Codec interface (`client.Codec`).** `ContentType()` and `Accept()` are
  collapsed into a single `MediaType() string`. Response-side compatibility
  moves into a new `MatchesResponse(contentType string) bool` so each codec
  owns its matching rules — the default `JSONCodec` matches `application/json`
  and any `+json` vendor suffix. Legacy helpers (`defaultCodec`,
  `isCodecResponse`, `typedResponseAccept`) are removed; `doJSON` no longer
  takes the operation's accept list because the codec's `MediaType()` drives
  the `Accept` header. Any third-party `client.Codec` implementation must
  adopt the new shape.
- **`.servergenignore` removed.** Use `server.ignore` in `tyche.json` instead.
  No deprecation period; the file and its loader are gone.

### Features

- **`tyche.json` config file** (sqlc-style). Place at the project root; the
  CLI discovers it by walking up from cwd to the first `go.mod`. Flags always
  override file values. Use `--config <path>` or `TYCHE_CONFIG=<path>` to
  point at a non-default file. Loader is `internal/config/`, stdlib
  `encoding/json` only — no new dependencies.
- **`tyche init`** scaffolds a `tyche.json` next to `go.mod`. Refuses to
  overwrite without `--force`. Prompts for the client module; `--yes` plus
  the right flags makes it non-interactive.
- **`tyche config show`** prints the resolved config (path, every value,
  source). `--json` for machine-readable output.
- **Multipart request support** in generated clients. Operations with
  `multipart/form-data` request bodies generate `form`, `file`, and `files`
  input fields. File inputs use the generated `client.File` type.
- **Strict 415 / 406 server-side.** JSON and multipart request bodies return
  415 for unsupported `Content-Type`; JSON and SSE responses return 406 when
  the request `Accept` header does not allow the produced media type.

## [1.1.1](https://github.com/WebDeveloperBen/tyche/compare/v1.1.0...v1.1.1) (2026-06-08)


### Bug Fixes

* **ci:** use latest versions of the actions ([df5ba14](https://github.com/WebDeveloperBen/tyche/commit/df5ba14d3017177b1ce208e5921d05b2c363f4e9))

## [1.1.0](https://github.com/WebDeveloperBen/tyche/compare/v1.0.0...v1.1.0) (2026-06-08)


### Features

* create clientgen for other golang consumers ([79f0670](https://github.com/WebDeveloperBen/tyche/commit/79f0670f393f6ae31eb85eb4d3641fc7234dc8d5))


### Bug Fixes

* correctness issues and race conditions across server and clientgen ([afb430e](https://github.com/WebDeveloperBen/tyche/commit/afb430e57bf3b7f9f30b8a22ca1dca043d42bbd6))
* integration tests to be self dependant ([1f37f55](https://github.com/WebDeveloperBen/tyche/commit/1f37f552a6e2a83a0ea5efc4786e1493badbbde4))

## 1.0.0 (2026-06-08)


### Features

* add go modernise and pre commit hooks and then run it across repo ([68e92f7](https://github.com/WebDeveloperBen/tyche/commit/68e92f7bb1619394d09a6029c088bf84e00c0f70))
* add streaming handler support ([81400e6](https://github.com/WebDeveloperBen/tyche/commit/81400e6b41456a3245cb10fa272646102cab1482))
* allow easy registration of middleware ([a952315](https://github.com/WebDeveloperBen/tyche/commit/a9523154842ef5459946853af485506264b814ec))


### Bug Fixes

* compression scope and responsibilities ([4cd3218](https://github.com/WebDeveloperBen/tyche/commit/4cd3218c3eb4345e4fe263089cf9ec601522c694))
* performance and correctness fixes ([b52b84e](https://github.com/WebDeveloperBen/tyche/commit/b52b84eaaa514275e8524a7dd4ad22aed3e53a58))
* security weaknesses ([f1a783d](https://github.com/WebDeveloperBen/tyche/commit/f1a783dcb0c9fa7a6bf99bb039b3481c6d03ebc2))
