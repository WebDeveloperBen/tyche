module github.com/webdeveloperben/tyche

go 1.25.5

require (
	github.com/alecthomas/kong v1.15.0
	github.com/andybalholm/brotli v1.2.1
	github.com/google/uuid v1.6.0
	golang.org/x/tools v0.47.0
)

require github.com/xyproto/randomstring v1.2.0 // indirect

require (
	github.com/bitfield/gotestdox v0.2.2 // indirect
	github.com/dnephin/pflag v1.0.7 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/telemetry v0.0.0-20260625142307-59b4966ccb57 // indirect
	golang.org/x/term v0.35.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	golang.org/x/vuln v1.5.0 // indirect
	gotest.tools/gotestsum v1.13.0 // indirect
	mvdan.cc/gofumpt v0.10.0 // indirect
)

tool (
	golang.org/x/tools/go/analysis/passes/modernize/cmd/modernize
	golang.org/x/vuln/cmd/govulncheck
	gotest.tools/gotestsum
	mvdan.cc/gofumpt
)
