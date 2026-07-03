1. Diagnosis of the current dependency situation
   The biggest finding — and the one that should reframe your whole plan — is that the dependency explosion is not Cobra's fault. It is the comparison build-tagged benchmark.
   server/benchmarks_router_test.go:1 is gated by //go:build comparison. It imports gin, chi, huma/humachi, and go-playground/validator/v10. validator/v10's own test file imports gofiber, gorilla/mux, echo, fasthttp, httprouter, bunrouter, fasthttp, goldmark, mattn/go-runewidth, valyala/fasttemplate, etc. Because go.mod is the union of all build tags, those modules get recorded as your direct deps. That is why gin, chi, and huma show up as direct deps even though zero production code touches them.
   Verifying what production code actually pulls in:
   Direct dep Production usage
   spf13/cobra servergen/cli/root.go, clientgen/cli/client.go — only the CLI
   golang.org/x/tools servergen (go/packages for codegen)
   andybalholm/brotli server/plugins/compressor.go (real)
   google/uuid server/plugins/requestid.go (real)
   gin-gonic/gin Only the comparison benchmark
   go-chi/chi/v5 Only the comparison benchmark
   danielgtaylor/huma/v2 Only the comparison benchmark
   Run on the CLI binary directly (go list -deps ./cmd/tyche | grep -v stdlib | sort -u) shows the runtime CLI closure is:

- github.com/spf13/cobra
- github.com/spf13/pflag (transitive)
- golang.org/x/tools/go/packages (+ internal/\*)
- golang.org/x/sync/errgroup (transitive via x/tools)
- golang.org/x/mod/semver (transitive via x/tools)
  That is it. The 81-module graph in your repo is mostly validator/v10's test transitive graph bleeding in via the comparison build tag.
  The single highest-leverage move is to detach the comparison benchmark from the main module. That alone drops your real dep set by ~75% with zero behavior change to your code or to go.mod semantics for downstream consumers.

2. Recommended target dependency set
   Status Module Why
   Must keep golang.org/x/tools Drives go/packages for codegen. Not at runtime — only when generating. Already non-runtime in practice.
   Must keep andybalholm/brotli Real value for a production HTTP framework; isolated to one plugin.
   Must keep google/uuid Real value in requestid plugin.
   Replace Cobra with github.com/alecthomas/kong (recommended) Struct-based, smaller dep than cobra+pflag, auto-generates help/completion.
   Remove from direct gin, chi, huma Move into a benchmarks/comparison/ sub-module (see migration plan).
   Add only if needed golang.org/x/term For password-style prompts in init (you do not currently need it).
   Skip All Charmbracelet packages See §5.
   Keep stdlib flag, flag/sub (if Go ≥1.22), encoding/json Config, JSON, paths, exec.
   Indirect deps you'll automatically lose once you clean up: bytedance/sonic, bytedance/gopkg, goccy/go-json, goccy/go-yaml, mattn/go-isatty, pflag, pelletier/go-toml/v2, ugorji/go/codec, go-playground/_ (all of it), json-iterator/go, quic-go/_, mousetrap, cpuguy83/go-md2man, davecgh/go-spew, pmezard/go-difflib, stretchr/testify (in the new sub-module), valyala/_, gofiber/fiber, gorilla/mux, labstack/echo, julienschmidt/httprouter, uptrace/bunrouter, kr/pretty, kr/text, rsc.io/pdf, rogpeppe/go-internal, golang/protobuf, stretchr/objx, evanphx/json-patch/v5, fxamacker/cbor/v2, xyproto/randomstring, leodido/go-urn, cloudwego/base64x, modern-go/_, gabriel-vasile/mimetype, mattn/go-colorable, mattn/go-runewidth, golang.org/x/telemetry, jordanlewis/gcassert, yuin/goldmark, russross/blackfriday/v2, danielgtaylor/mexpr, danielgtaylor/shorthand/v2, go.yaml.in/yaml/v3, gopkg.in/yaml.v3, gopkg.in/check.v1, clipperhouse/uax29/v2, x448/float16, golang.org/x/arch, golang.org/x/crypto, golang.org/x/term, klauspost/cpuid/v2, golang.org/x/text, google.golang.org/protobuf, golang.org/x/sys, twitchyliquid64/golang-asm, go.uber.org/mock, valyala/bytebufferpool.
   You go from ~78 transitive modules (your current effective closure) to about ~12 — cobra-or-kong + pflag-or-not + x/tools internals + brotli + uuid + a few x/{mod,sync,net,text,tools} stdlib-adjacent.
3. Recommended CLI architecture
   tyche/
   ├── cmd/
   │ ├── tyche/ # thin main: wires Kong → cmd.Execute
   │ │ └── main.go
   │ └── api/ # existing local dev harness (unchanged)
   ├── internal/
   │ ├── cli/ # CLI surface: Kong command tree, flag binding, dispatch
   │ │ ├── root.go # the Kong struct + root command
   │ │ ├── init.go # `tyche init`
   │ │ ├── config.go # `tyche config show`
   │ │ ├── generate.go # `tyche generate`
   │ │ ├── clean.go # `tyche clean`
   │ │ ├── build.go # `tyche build`
   │ │ ├── run.go # `tyche run`
   │ │ ├── test.go # `tyche test`
   │ │ ├── client.go # `tyche client`
   │ │ ├── completion.go # `tyche completion` (Kong already provides)
   │ │ └── version.go # `tyche version`
   │ ├── app/ # use-case orchestrators: pure functions, no CLI types
   │ │ ├── codegen.go # wrap servergen.WriteGeneratedFiles
   │ │ ├── worktree.go # wrap withGeneratedWorktree
   │ │ ├── scaffold.go # wrap runInit's logic
   │ │ └── clientgen.go # wrap clientgen.Generate
   │ ├── config/ # already exists, no change
   │ ├── output/ # formatting layer: human, json, quiet modes
   │ │ ├── printer.go # Printer interface + Human/JSON/Quiet impls
   │ │ └── errors.go # error formatting (problem+json style for CLI)
   │ ├── ui/ # terminal interaction primitives
   │ │ ├── prompt.go # stdin/stdout prompt loop, isatty check
   │ │ └── term.go # TTY detection, width, color toggling
   │ └── version/ # build info populated via -ldflags
   │ └── version.go
   ├── server/ # unchanged
   ├── servergen/ # the lib (no CLI types leak out)
   │ └── ... # rename servergen/cli → callers use internal/cli+internal/app
   ├── clientgen/ # the lib (no CLI types leak out)
   │ └── ...
   ├── benchmarks/
   │ └── comparison/ # NEW — a sub-module; only thing that depends on gin/chi/huma/validator
   │ ├── go.mod
   │ ├── go.sum
   │ └── router_test.go
   ├── go.mod
   ├── go.sum
   └── ...
   The key invariant: servergen/ and clientgen/ libraries must not import internal/cli/. Today they don't, but the servergen/cli/ and clientgen/cli/ packages live next to the libs. With the new structure, the Cobra/Kong-specific glue lives in internal/cli/, and internal/app/ is the library-facing facade. That way:

- A user embedding tyche in their own tooling can call internal/app.Scaffold(...) directly with no CLI types.
- The CLI is the only thing that knows about Kong.
- The libs (servergen, clientgen, server) never gain a CLI dependency.

4. Step-by-step migration plan
1. Isolate the comparison benchmark (1 PR, low risk). Create benchmarks/comparison/go.mod. Move server/benchmarks_router_test.go to benchmarks/comparison/router_test.go. Add a tiny benchmarks/comparison/tyche.go that does replace github.com/webdeveloperben/tyche => ../.. so the sub-module can import the local server package. Remove the //go:build comparison tag (the sub-module's go.mod enforces the boundary now). Delete from main go.mod/go.sum: gin, chi, huma, validator/v10 and ~70 transitives.
1. Add a build-time path to the new CLI (no behavior change). Introduce internal/cli/, internal/app/, internal/output/, internal/ui/, internal/version/. Implement the new CLI on top of Kong. Wire cmd/tyche/main.go to the new entry point. Keep servergen/cli and clientgen/cli for the duration of the migration as compatibility shims (they just re-export the new struct). Mark them Deprecated in the doc comment.
1. Switch cmd/tyche/main.go over. It now calls the new internal/cli.Run(args). Keep the same exit codes. Run task tests and task verify.
1. Update CI: add a task verify:cli that runs go run ./cmd/tyche --help, --version, completion bash, and one subcommand per top-level verb through a tmpdir with a fake tyche.json, asserting non-zero exit and expected output on a bad input.
1. Delete the old CLI packages in the next semver. servergen/cli and clientgen/cli are removed; their callers have moved to the new structure.
1. (Optional) Make brotli opt-in. compressor.go is already a separate plugin. If you want the smallest possible binary, you can split server/plugins/compressor into a sub-module with brotli as a build tag, or accept the one dep. I would not bother — the dep is focused and the feature is real value.
1. Recommended CLI: Kong, with example
   Kong is a good fit: struct-based definitions, one dep, smaller than cobra+pflag, generates help and completion, and removes a layer of cmd.Flags().StringVar(&x, ...) noise.
   internal/cli/root.go:
   package cli

import (
"github.com/alecthomas/kong"

    "github.com/webdeveloperben/tyche/internal/app"
    "github.com/webdeveloperben/tyche/internal/output"
    "github.com/webdeveloperben/tyche/internal/version"

)

// CLI is the single source of truth for `tyche`'s surface. Each subcommand
// is a method receiver so the parent's flags (--config, --root, --quiet) are
// inherited automatically; Kong resolves precedence: command-line > struct
// default > env (via the "env" tag).
type CLI struct {
GlobalFlags
Init InitCmd `cmd:"" help:"Scaffold a tyche.json config file in the project root."`
Config ConfigCmd `cmd:"" help:"Inspect and validate the resolved tyche.json."`
Generate GenerateCmd `cmd:"" help:"Generate typed route codecs into the working tree."`
Clean CleanCmd `cmd:"" help:"Remove generated route codec files from the working tree."`
Build BuildCmd `cmd:"build" help:"Build a package from a temporary generated worktree."`
Run RunCmd `cmd:"run"   help:"Run a package from a temporary generated worktree."`
Test TestCmd `cmd:"test"  help:"Run tests from a temporary generated worktree."`
Client ClientCmd `cmd:"" help:"Regenerate the typed client from a tyche OpenAPI spec."`
Version VersionCmd `cmd:"" help:"Print version, build commit, and Go version."`
Completion CompletionCmd `cmd:"" hidden:"" help:"Print shell completion script (bash|zsh|fish|powershell)."`
}

type GlobalFlags struct {
Config string `help:"Path to a tyche.json config file (overrides discovery)." short:"c" env:"TYCHE_CONFIG"`
Root string `help:"Project root." default:""`
Quiet bool `help:"Suppress the 'using config ...' info line." short:"q"`
Output string `help:"Output format: human (default) or json." enum:"human,json" default:"human"`
}

func Run(args []string) error {
var cli CLI
parser, err := kong.New(&cli,
kong.Name("tyche"),
kong.Description("Generate typed Go HTTP servers and clients from an OpenAPI spec."),
kong.UsageOnError(),
kong.Vars{"version": version.String()},
kong.ConfigureHelp(kong.HelpOptions{
Compact: true,
Indent: " ",
NoExpandSubcommands: false,
}),
)
if err != nil {
return err
}
kctx, err := parser.Parse(args)
if err != nil {
return err // kong already printed usage on parse error
}
if err := kctx.Run(&cli.GlobalFlags); err != nil {
output.PrintError(kctx, err)
return err
}
return nil
}
internal/cli/init.go:
package cli

import (
"errors"
"fmt"
"os"
"path/filepath"

    "github.com/webdeveloperben/tyche/internal/app"
    "github.com/webdeveloperben/tyche/internal/config"
    "github.com/webdeveloperben/tyche/internal/ui"

)

type InitCmd struct {
Module string `help:"Go module path for the generated client (skips the prompt)." short:"m"`
Spec string `help:"Path to the OpenAPI document (skips the prompt)." default:"./api/openapi.json"`
TypeNaming string `help:"Generated type naming strategy." enum:"structural,operation-scoped" default:"structural"`
Force bool `help:"Overwrite an existing tyche.json." short:"f"`
Yes bool `help:"Skip prompts; require all answers via flags." short:"y"`
}

func (c *InitCmd) Run(globals *GlobalFlags) error {
root, err := resolveRoot(globals.Root)
if err != nil {
return err
}
if c.Module == "" && !c.Yes {
if !ui.IsTerminal(os.Stdin) {
return errors.New("non-interactive shell; pass --module, --spec, and --type-naming to scaffold")
}
prompt := ui.Prompt{Out: globals.stderr(), In: os.Stdin}
m, err := prompt.Ask("Client module path (e.g. github.com/acme/api/client):")
if err != nil {
return fmt.Errorf("read module: %w", err)
}
c.Module = m
}
if c.Module == "" {
return errors.New("client module path is required (use --module or answer the prompt)")
}
return app.Scaffold(app.ScaffoldOptions{
Root: root,
Module: c.Module,
Spec: c.Spec,
TypeNaming: c.TypeNaming,
Force: c.Force,
})
}

func (g *GlobalFlags) stderr() *os.File { return os.Stderr }
internal/output/printer.go:
package output

import (
"encoding/json"
"io"
"os"
)

// Printer is the output sink for command results. Implementations: Human (the
// default column-aligned text), JSON (machine-readable, suitable for piping
// into jq), and Quiet (only the requested data, no decorations).
type Printer interface {
Info(msg string, args ...any)
Result(v any) error
Error(err error)
}

func New(mode string, stdout, stderr io.Writer) Printer {
switch mode {
case "json":
return &jsonPrinter{out: stdout, err: stderr}
case "quiet":
return &quietPrinter{out: stdout}
default:
return &humanPrinter{out: stdout, err: stderr}
}
}

func PrintError(format string, args ...any) {
fmt.Fprintf(os.Stderr, "tyche: error: "+format+"\n", args...)
} 6. Decision checklist: stdlib vs Cobra vs Kong
If you answer "yes" to most of these Pick
≤2 commands, both trivial, no subcommands stdlib flag
I want zero third-party CLI deps and I'll write a 200-line dispatcher stdlib flag
I want minimum effort, mature docs, lots of Stack Overflow Cobra
I need plugins, hooks, persistent flag inheritance, and many subcommand groups Cobra
I want struct-typed commands, no Flags().StringVar(&x, ...) boilerplate Kong
I want shell completion for free without writing it Kong or Cobra (both have it)
I want auto-generated man pages Cobra (via cobra/doc) — Kong does not
I want a tiny dep closure Kong (1 dep) or stdlib (0)
For tyche, the answer is Kong. You have 8 subcommands, no nested subcommands, no plugins, and the commands are mostly data + dispatch. Kong's struct model maps 1:1 to your CLI and removes a layer of RunE closures. 7. Risks and tradeoffs

- Kong is less ubiquitous than Cobra. Smaller community. If contributors are coming from Kubernetes/Docker world, Cobra is what they know. The CLI is small enough that this is a minor concern.
- Shell completion scripts. Kong supports it (kong completion bash), but it is less documented than Cobra's tyche completion bash. If first-class completion matters, smoke-test it before committing.
- The comparison benchmark move is the real win, not the CLI swap. If you only do one thing, do the sub-module split. Everything else is polish.
- Brotli stays. It is one focused dep, isolated to one plugin, and gives a real production feature. Removing it is a regression in user value.
- Pflag stays if you stay on Cobra. pflag is part of the spf13/\* family and is well-maintained; do not view it as "bloat" by itself.
- Output modes. Adding human|json|quiet is a real DX win but also a real surface to test. If you do it, do it once in internal/output and have every command go through it — don't sprinkle format strings.
- Removing mousetrap (Windows message for double-clicked binaries). Cobra pulls it in. Kong does not. If you only ship for macOS/Linux, both are noise.
- The golang.org/x/tools dep is large (a few MB of stdlib internal types). It is unavoidable for go/packages. Keep it, but be aware that codegen via go/packages requires users to have a Go toolchain available — which is already true.
  Bottom line: the dep bloat is a build-tag leak, not a Cobra problem. Fix that first; switch the CLI to Kong while you're in there; do not add Charmbracelet; keep brotli; let internal/output and internal/app carry the new architecture.
