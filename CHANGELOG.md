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

## [3.0.0](https://github.com/WebDeveloperBen/tyche/compare/v2.0.2...v3.0.0) (2026-07-12)


### ⚠ BREAKING CHANGES

* unify CLI as tyche and add tyche.json config

### Features

* add codex interface for typed errors and response parsing ([af90d80](https://github.com/WebDeveloperBen/tyche/commit/af90d801f1ffac9ca7d72b311dd0b4b641fcb0d5))
* add go modernise and pre commit hooks and then run it across repo ([68e92f7](https://github.com/WebDeveloperBen/tyche/commit/68e92f7bb1619394d09a6029c088bf84e00c0f70))
* add multipart support implementation in servergen ([f04cdcf](https://github.com/WebDeveloperBen/tyche/commit/f04cdcf0dedebebc66beb56f8f1f9b310e6c84f7))
* add streaming handler support ([81400e6](https://github.com/WebDeveloperBen/tyche/commit/81400e6b41456a3245cb10fa272646102cab1482))
* add typed multipart support ([19f1977](https://github.com/WebDeveloperBen/tyche/commit/19f197779c5cb45d0764567ccb9354fa1bd584a5))
* allow easy registration of middleware ([a952315](https://github.com/WebDeveloperBen/tyche/commit/a9523154842ef5459946853af485506264b814ec))
* clean up typegen for clientgen ([38dadc7](https://github.com/WebDeveloperBen/tyche/commit/38dadc71050ea24e1aeb2169a155166b22d8b821))
* create clientgen for other golang consumers ([79f0670](https://github.com/WebDeveloperBen/tyche/commit/79f0670f393f6ae31eb85eb4d3641fc7234dc8d5))
* create type naming strategies for client genearted code ([7e03534](https://github.com/WebDeveloperBen/tyche/commit/7e035344009072de564a18078e17a20733682bc1))
* generate request support from input fields ([4ea9505](https://github.com/WebDeveloperBen/tyche/commit/4ea950571d368ad13c732bd957acc16432ec648d))
* strengthen typed runtime of text event streams ([3adde76](https://github.com/WebDeveloperBen/tyche/commit/3adde76ca93b81bb818c221d60e17a1d398caff6))
* unify CLI as tyche and add tyche.json config ([1cb3ad8](https://github.com/WebDeveloperBen/tyche/commit/1cb3ad847d3c9d13eb0109e8fb7426c5589fa5e8))


### Bug Fixes

* all responses aren't now typed as json ([14c8019](https://github.com/WebDeveloperBen/tyche/commit/14c8019199c486da17cc643f387c6bdba76e837e))
* all types to be created in main ([d00cc61](https://github.com/WebDeveloperBen/tyche/commit/d00cc6175a80b3c3407a5d6c2ebf131524ee2fa5))
* **ci:** add lefthook to catch out of date go.mod ([a720ed7](https://github.com/WebDeveloperBen/tyche/commit/a720ed79fa09d3602c812d2430f684902be8cdf7))
* **ci:** gorealeser ([a4f0758](https://github.com/WebDeveloperBen/tyche/commit/a4f075857cdf753a5bc328a364ffc4e2a8490d6e))
* **ci:** prevent cache poisoning in ci ([4d208d7](https://github.com/WebDeveloperBen/tyche/commit/4d208d7a1249f7abbb2d3e7cc06df0ade6219a61))
* **ci:** use latest versions of the actions ([df5ba14](https://github.com/WebDeveloperBen/tyche/commit/df5ba14d3017177b1ce208e5921d05b2c363f4e9))
* **ci:** use ref for checkout ([f5ab9cd](https://github.com/WebDeveloperBen/tyche/commit/f5ab9cdd2397b3d07c1b84b82ff0afd26cdc4f6e))
* **ci:** zizmor expanded braces finding ([4e846c3](https://github.com/WebDeveloperBen/tyche/commit/4e846c3845e34604d12bdaa8d6f13fcabdd544f3))
* clean up plugin middleware register ([b848ff9](https://github.com/WebDeveloperBen/tyche/commit/b848ff906f5b15fa4e63c049f3eace60218f0638))
* compression scope and responsibilities ([4cd3218](https://github.com/WebDeveloperBen/tyche/commit/4cd3218c3eb4345e4fe263089cf9ec601522c694))
* contract break in generated code ([84337dc](https://github.com/WebDeveloperBen/tyche/commit/84337dc44813a5cafd0a9227aae97d1a5b4ce9dc))
* correctness issues and race conditions across server and clientgen ([afb430e](https://github.com/WebDeveloperBen/tyche/commit/afb430e57bf3b7f9f30b8a22ca1dca043d42bbd6))
* correctness linting ([4da019e](https://github.com/WebDeveloperBen/tyche/commit/4da019e1d5f72abcabdd5e3afeff53768554dbdb))
* devtooling flow ([a017348](https://github.com/WebDeveloperBen/tyche/commit/a0173485aa0380ef1a19038174076f2f7f3028f1))
* harden CLI error handling and output after refactor review ([8dd802e](https://github.com/WebDeveloperBen/tyche/commit/8dd802ef11c8a91217426b378406f833ddc9dfee))
* integration tests to be self dependant ([1f37f55](https://github.com/WebDeveloperBen/tyche/commit/1f37f552a6e2a83a0ea5efc4786e1493badbbde4))
* linting and add commands to taskfile to run easily ([f684bbc](https://github.com/WebDeveloperBen/tyche/commit/f684bbc86b28efe060e09e7f3481f48015438c96))
* performance and correctness fixes ([b52b84e](https://github.com/WebDeveloperBen/tyche/commit/b52b84eaaa514275e8524a7dd4ad22aed3e53a58))
* put tools into mise so they install in ci ([9a25019](https://github.com/WebDeveloperBen/tyche/commit/9a2501938b528056a6a58e1e9ec7d7289a42a63d))
* reg bug ([603e460](https://github.com/WebDeveloperBen/tyche/commit/603e460b23439e8a65700d89eac01a6be8979a1a))
* security weaknesses ([f1a783d](https://github.com/WebDeveloperBen/tyche/commit/f1a783dcb0c9fa7a6bf99bb039b3481c6d03ebc2))
* typegen ([61f88ad](https://github.com/WebDeveloperBen/tyche/commit/61f88ade0ded548b91a9d803f2d4150265640166))
* update go mod ([0ec22c4](https://github.com/WebDeveloperBen/tyche/commit/0ec22c4487a039400aa2210edb915b77ccf63245))
* update memory footprint of structs ([08a82b4](https://github.com/WebDeveloperBen/tyche/commit/08a82b48d10caeeb5dfa6897b5271836947f3571))
* vuln in golang std lib bump to latest ([d079fd0](https://github.com/WebDeveloperBen/tyche/commit/d079fd012d001055c3835c4e53afe5f723c9df8e))
* ws forwarding bug ([1764584](https://github.com/WebDeveloperBen/tyche/commit/1764584310aabadebcc943761e637a0a2b32be7c))

## [2.0.2](https://github.com/WebDeveloperBen/tyche/compare/v2.0.1...v2.0.2) (2026-07-12)


### Bug Fixes

* **ci:** gorealeser ([a4f0758](https://github.com/WebDeveloperBen/tyche/commit/a4f075857cdf753a5bc328a364ffc4e2a8490d6e))
* **ci:** zizmor expanded braces finding ([4e846c3](https://github.com/WebDeveloperBen/tyche/commit/4e846c3845e34604d12bdaa8d6f13fcabdd544f3))

## [2.0.1](https://github.com/WebDeveloperBen/tyche/compare/v2.0.0...v2.0.1) (2026-07-12)


### Bug Fixes

* **ci:** add lefthook to catch out of date go.mod ([a720ed7](https://github.com/WebDeveloperBen/tyche/commit/a720ed79fa09d3602c812d2430f684902be8cdf7))
* **ci:** prevent cache poisoning in ci ([4d208d7](https://github.com/WebDeveloperBen/tyche/commit/4d208d7a1249f7abbb2d3e7cc06df0ade6219a61))
* correctness linting ([4da019e](https://github.com/WebDeveloperBen/tyche/commit/4da019e1d5f72abcabdd5e3afeff53768554dbdb))
* put tools into mise so they install in ci ([9a25019](https://github.com/WebDeveloperBen/tyche/commit/9a2501938b528056a6a58e1e9ec7d7289a42a63d))
* update go mod ([0ec22c4](https://github.com/WebDeveloperBen/tyche/commit/0ec22c4487a039400aa2210edb915b77ccf63245))
* vuln in golang std lib bump to latest ([d079fd0](https://github.com/WebDeveloperBen/tyche/commit/d079fd012d001055c3835c4e53afe5f723c9df8e))

## [2.0.0](https://github.com/WebDeveloperBen/tyche/compare/v1.1.1...v2.0.0) (2026-07-03)


### ⚠ BREAKING CHANGES

* unify CLI as tyche and add tyche.json config

### Features

* add codex interface for typed errors and response parsing ([af90d80](https://github.com/WebDeveloperBen/tyche/commit/af90d801f1ffac9ca7d72b311dd0b4b641fcb0d5))
* add multipart support implementation in servergen ([f04cdcf](https://github.com/WebDeveloperBen/tyche/commit/f04cdcf0dedebebc66beb56f8f1f9b310e6c84f7))
* add typed multipart support ([19f1977](https://github.com/WebDeveloperBen/tyche/commit/19f197779c5cb45d0764567ccb9354fa1bd584a5))
* clean up typegen for clientgen ([38dadc7](https://github.com/WebDeveloperBen/tyche/commit/38dadc71050ea24e1aeb2169a155166b22d8b821))
* create type naming strategies for client genearted code ([7e03534](https://github.com/WebDeveloperBen/tyche/commit/7e035344009072de564a18078e17a20733682bc1))
* generate request support from input fields ([4ea9505](https://github.com/WebDeveloperBen/tyche/commit/4ea950571d368ad13c732bd957acc16432ec648d))
* strengthen typed runtime of text event streams ([3adde76](https://github.com/WebDeveloperBen/tyche/commit/3adde76ca93b81bb818c221d60e17a1d398caff6))
* unify CLI as tyche and add tyche.json config ([1cb3ad8](https://github.com/WebDeveloperBen/tyche/commit/1cb3ad847d3c9d13eb0109e8fb7426c5589fa5e8))


### Bug Fixes

* all responses aren't now typed as json ([14c8019](https://github.com/WebDeveloperBen/tyche/commit/14c8019199c486da17cc643f387c6bdba76e837e))
* all types to be created in main ([d00cc61](https://github.com/WebDeveloperBen/tyche/commit/d00cc6175a80b3c3407a5d6c2ebf131524ee2fa5))
* clean up plugin middleware register ([b848ff9](https://github.com/WebDeveloperBen/tyche/commit/b848ff906f5b15fa4e63c049f3eace60218f0638))
* contract break in generated code ([84337dc](https://github.com/WebDeveloperBen/tyche/commit/84337dc44813a5cafd0a9227aae97d1a5b4ce9dc))
* devtooling flow ([a017348](https://github.com/WebDeveloperBen/tyche/commit/a0173485aa0380ef1a19038174076f2f7f3028f1))
* harden CLI error handling and output after refactor review ([8dd802e](https://github.com/WebDeveloperBen/tyche/commit/8dd802ef11c8a91217426b378406f833ddc9dfee))
* linting and add commands to taskfile to run easily ([f684bbc](https://github.com/WebDeveloperBen/tyche/commit/f684bbc86b28efe060e09e7f3481f48015438c96))
* reg bug ([603e460](https://github.com/WebDeveloperBen/tyche/commit/603e460b23439e8a65700d89eac01a6be8979a1a))
* typegen ([61f88ad](https://github.com/WebDeveloperBen/tyche/commit/61f88ade0ded548b91a9d803f2d4150265640166))
* update memory footprint of structs ([08a82b4](https://github.com/WebDeveloperBen/tyche/commit/08a82b48d10caeeb5dfa6897b5271836947f3571))
* ws forwarding bug ([1764584](https://github.com/WebDeveloperBen/tyche/commit/1764584310aabadebcc943761e637a0a2b32be7c))

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
