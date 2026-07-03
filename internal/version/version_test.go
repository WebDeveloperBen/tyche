package version

import (
	"strings"
	"sync"
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

func TestString_HandlesMissingCommit(t *testing.T) {
	// Reset module-level state for this test.
	origCommit := Commit
	Commit = ""
	defer func() { Commit = origCommit }()

	once = sync.Once{}
	s := String()
	if !strings.Contains(s, "tyche") {
		t.Errorf("String() = %q, missing 'tyche'", s)
	}
	if strings.Contains(s, ",") && !strings.HasPrefix(s, "tyche ") {
		// If there's no commit, the format is "tyche <ver> (built ... with goX.Y.Z)".
		// A leading "tyche X (" is fine; we just want to confirm the build line
		// still renders without a commit.
		t.Logf("String() with no commit = %q", s)
	}
}
