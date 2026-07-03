package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestParseMode_Defaults(t *testing.T) {
	cases := map[string]Mode{
		"":        ModeHuman,
		"human":   ModeHuman,
		"json":    ModeJSON,
		"quiet":   ModeQuiet,
		"h":       ModeHuman,
		"j":       ModeJSON,
		"q":       ModeQuiet,
		"unknown": ModeHuman,
	}
	for in, want := range cases {
		if got := ParseMode(in); got != want {
			t.Errorf("ParseMode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanPrinter_ResultWritesString(t *testing.T) {
	var out, errOut bytes.Buffer
	p := New(ModeHuman, &out, &errOut)
	if err := p.Result("hello world"); err != nil {
		t.Fatalf("Result: %v", err)
	}
	if out.String() != "hello world\n" {
		t.Errorf("stdout = %q, want %q", out.String(), "hello world\n")
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}

func TestHumanPrinter_InfoWritesToStderr(t *testing.T) {
	var out, errOut bytes.Buffer
	p := New(ModeHuman, &out, &errOut)
	p.Info("using config %s", "/path/to/tyche.json")
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty (info should go to stderr)", out.String())
	}
	want := "tyche: using config /path/to/tyche.json\n"
	if errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestHumanPrinter_ErrorRendersOneLine(t *testing.T) {
	var out, errOut bytes.Buffer
	p := New(ModeHuman, &out, &errOut)
	p.Error(errors.New("bad config"))
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty (error should go to stderr)", out.String())
	}
	if !strings.Contains(errOut.String(), "tyche: error: bad config") {
		t.Errorf("stderr = %q, missing error text", errOut.String())
	}
}

func TestJSONPrinter_ResultEmitsJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	p := New(ModeJSON, &out, &errOut)
	type payload struct {
		Path string `json:"path"`
		Ver  int    `json:"version"`
	}
	if err := p.Result(&payload{Path: "/x/tyche.json", Ver: 1}); err != nil {
		t.Fatalf("Result: %v", err)
	}
	var got payload
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v (raw: %s)", err, out.String())
	}
	if got.Path != "/x/tyche.json" || got.Ver != 1 {
		t.Errorf("got %+v, want {/x/tyche.json 1}", got)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty for non-info JSON result", errOut.String())
	}
}

func TestJSONPrinter_InfoEmitsJSONLine(t *testing.T) {
	var out, errOut bytes.Buffer
	p := New(ModeJSON, &out, &errOut)
	p.Info("using config %s", "/x")
	var line map[string]any
	if err := json.Unmarshal(errOut.Bytes(), &line); err != nil {
		t.Fatalf("unmarshal: %v (raw: %s)", err, errOut.String())
	}
	if line["level"] != "info" {
		t.Errorf("level = %v, want info", line["level"])
	}
	if msg, _ := line["message"].(string); !strings.Contains(msg, "using config /x") {
		t.Errorf("message = %q, want it to contain 'using config /x'", msg)
	}
}

func TestJSONPrinter_ErrorEmitsProblemJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	p := New(ModeJSON, &out, &errOut)
	p.Error(errors.New("boom"))
	var line map[string]any
	if err := json.Unmarshal(errOut.Bytes(), &line); err != nil {
		t.Fatalf("unmarshal: %v (raw: %s)", err, errOut.String())
	}
	if line["type"] != "about:blank" || line["title"] != "error" {
		t.Errorf("got %+v, want a problem+json shape", line)
	}
	if line["detail"] != "boom" {
		t.Errorf("detail = %v, want boom", line["detail"])
	}
}

func TestQuietPrinter_DropsInfo(t *testing.T) {
	var out, errOut bytes.Buffer
	p := New(ModeQuiet, &out, &errOut)
	p.Info("anything")
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("quiet mode should drop info, got out=%q err=%q", out.String(), errOut.String())
	}
}

func TestQuietPrinter_WritesStringResultsOnly(t *testing.T) {
	var out bytes.Buffer
	p := New(ModeQuiet, &out, &out)
	if err := p.Result("just this"); err != nil {
		t.Fatalf("Result: %v", err)
	}
	if out.String() != "just this\n" {
		t.Errorf("stdout = %q, want %q", out.String(), "just this\n")
	}
	// Non-string results are dropped in quiet mode: callers needing
	// structured output should use --output=json.
	var out2 bytes.Buffer
	p2 := New(ModeQuiet, &out2, &out2)
	type payload struct{ A int }
	if err := p2.Result(&payload{A: 1}); err != nil {
		t.Fatalf("Result: %v", err)
	}
	if out2.Len() != 0 {
		t.Errorf("expected empty output for non-string quiet result, got %q", out2.String())
	}
}

func TestQuietPrinter_DropsErrors(t *testing.T) {
	var out, errOut bytes.Buffer
	p := New(ModeQuiet, &out, &errOut)
	p.Error(errors.New("dropped"))
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Errorf("quiet mode should drop errors, got out=%q err=%q", out.String(), errOut.String())
	}
}

func TestNew_NilWritersDefaultToDiscard(t *testing.T) {
	// Should not panic, should not write to anything visible.
	p := New(ModeHuman, nil, nil)
	p.Info("hello")
	if err := p.Result("world"); err != nil {
		t.Fatalf("Result: %v", err)
	}
	p.Error(errors.New("nope"))
}
