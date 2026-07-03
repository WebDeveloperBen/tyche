package version

import (
	"strings"
	"testing"
)

func TestGet_PopulatesGoVersion(t *testing.T) {
	i := Get()
	if i.Go == "" {
		t.Errorf("Get() did not populate Go field")
	}
	if !strings.HasPrefix(i.Go, "go") {
		t.Errorf("Go = %q, want go-prefixed runtime.Version()", i.Go)
	}
}

func TestString_ContainsKeyFields(t *testing.T) {
	s := String()
	for _, want := range []string{"tyche", "go"} {
		if !strings.Contains(s, want) {
			t.Errorf("String() = %q, missing %q", s, want)
		}
	}
}

func TestFormat_HandlesMissingCommit(t *testing.T) {
	// format is pure, so we test it with a synthetic Info rather than
	// mutating the package-level ldflag vars or resetting sync.Once.
	s := format(Info{Version: "1.2.3", BuiltBy: "test", Go: "go1.99"})
	if !strings.Contains(s, "tyche 1.2.3") {
		t.Errorf("format() = %q, missing version", s)
	}
	if strings.Contains(s, "(") && strings.Contains(s[:strings.Index(s, "(")], ",") {
		t.Errorf("format() with no commit should not include a commit clause: %q", s)
	}
	// Sanity: the summary should not carry a commit fragment when Commit is empty.
	if strings.Contains(s, ", built") {
		t.Errorf("format() with empty commit unexpectedly rendered a commit: %q", s)
	}
}

func TestFormat_IncludesShortCommit(t *testing.T) {
	s := format(Info{Version: "1.2.3", Commit: "0123456789abcdef", BuiltBy: "test", Go: "go1.99"})
	if !strings.Contains(s, "012345678") {
		t.Errorf("format() = %q, missing short commit", s)
	}
	if strings.Contains(s, "0123456789abcdef") {
		t.Errorf("format() = %q, commit should be shortened to 12 chars", s)
	}
}
