package validation_test

import (
	"testing"

	"github.com/webdeveloperben/tyche/server/validation"
)

func TestJSONPointer(t *testing.T) {
	tests := []struct {
		name  string
		want  string
		parts []string
	}{
		{name: "nil parts", parts: nil, want: ""},
		{name: "single empty part", parts: []string{""}, want: ""},
		{name: "single part", parts: []string{"foo"}, want: "/foo"},
		{name: "two parts", parts: []string{"foo", "bar"}, want: "/foo/bar"},
		{name: "three parts", parts: []string{"a", "b", "c"}, want: "/a/b/c"},
		{name: "tilde escape", parts: []string{"tilde~field"}, want: "/tilde~0field"},
		{name: "slash escape", parts: []string{"slash/field"}, want: "/slash~1field"},
		{name: "both escapes", parts: []string{"a~b/c"}, want: "/a~0b~1c"},
		{name: "skip empty parts", parts: []string{"a", "", "b"}, want: "/a/b"},
		{name: "leading empty part", parts: []string{"", "a"}, want: "/a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validation.JSONPointer(tt.parts...)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJoinPointer(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		want  string
		parts []string
	}{
		{name: "empty base uses JSONPointer", base: "", parts: []string{"foo"}, want: "/foo"},
		{name: "base with part", base: "/base", parts: []string{"field"}, want: "/base/field"},
		{name: "bare base with part", base: "base", parts: []string{"field"}, want: "base/field"},
		{name: "skip empty parts", base: "/base", parts: []string{"", "field"}, want: "/base/field"},
		{name: "multiple parts", base: "/root", parts: []string{"a", "b"}, want: "/root/a/b"},
		{name: "no parts", base: "/base", parts: nil, want: "/base"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validation.JoinPointer(tt.base, tt.parts...)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJoinPointerWithIndex(t *testing.T) {
	tests := []struct {
		name  string
		base  string
		want  string
		index int
	}{
		{name: "empty base", base: "", index: 0, want: "0"},
		{name: "empty base non-zero index", base: "", index: 5, want: "5"},
		{name: "with base", base: "/items", index: 0, want: "/items/0"},
		{name: "with base non-zero", base: "/items", index: 3, want: "/items/3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validation.JoinPointerWithIndex(tt.base, tt.index)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_Empty(t *testing.T) {
	var e validation.Error
	if !e.Empty() {
		t.Fatal("zero-value Error should be empty")
	}
	e.Add("/field", "required", "Field is required.")
	if e.Empty() {
		t.Fatal("Error with a problem should not be empty")
	}
}

func TestError_Empty_NilPointer(t *testing.T) {
	var e *validation.Error
	if !e.Empty() {
		t.Fatal("nil *Error should be empty")
	}
}

func TestError_Add(t *testing.T) {
	var e validation.Error
	e.Add("/name", "required", "Field is required.")
	if len(e.Problems) != 1 {
		t.Fatalf("expected 1 problem, got %d", len(e.Problems))
	}
	p := e.Problems[0]
	if p.Pointer != "/name" || p.Code != "required" || p.Message != "Field is required." {
		t.Errorf("unexpected problem: %+v", p)
	}
}

func TestError_Merge(t *testing.T) {
	a := &validation.Error{}
	a.Add("/a", "required", "msg a")

	b := &validation.Error{}
	b.Add("/b", "min", "msg b")

	a.Merge(b)
	if len(a.Problems) != 2 {
		t.Fatalf("expected 2 problems after merge, got %d", len(a.Problems))
	}
}

func TestError_Merge_NilSource(t *testing.T) {
	a := &validation.Error{}
	a.Add("/a", "required", "msg")
	a.Merge(nil)
	if len(a.Problems) != 1 {
		t.Fatalf("merge with nil should not change problems, got %d", len(a.Problems))
	}
}

func TestError_Error_Message(t *testing.T) {
	var e validation.Error
	if e.Error() != "validation failed" {
		t.Errorf("zero-value error message: got %q", e.Error())
	}

	e.Add("/field", "required", "Field is required.")
	if e.Error() != "Field is required." {
		t.Errorf("single problem message: got %q", e.Error())
	}

	e.Add("/other", "min", "Too short.")
	if got := e.Error(); got == "" {
		t.Error("multi-problem message should not be empty")
	}
}

func TestProblemConstructors(t *testing.T) {
	t.Run("RequiredProblem", func(t *testing.T) {
		p := validation.RequiredProblem("/name")
		if p.Pointer != "/name" || p.Code != "required" || p.Message == "" {
			t.Errorf("unexpected: %+v", p)
		}
	})

	t.Run("InvalidTypeProblem", func(t *testing.T) {
		p := validation.InvalidTypeProblem("/age")
		if p.Pointer != "/age" || p.Code != "invalid_type" || p.Message == "" {
			t.Errorf("unexpected: %+v", p)
		}
	})
}

func TestRuleProblem_Messages(t *testing.T) {
	tests := []struct {
		kind    validation.RuleKind
		subject validation.Subject
		code    string
		value   int
	}{
		{kind: validation.RuleMin, subject: validation.SubjectString, value: 5, code: "min"},
		{kind: validation.RuleMin, subject: validation.SubjectNumber, value: 0, code: "min"},
		{kind: validation.RuleMax, subject: validation.SubjectString, value: 10, code: "max"},
		{kind: validation.RuleMax, subject: validation.SubjectNumber, value: 100, code: "max"},
		{kind: validation.RuleLen, subject: validation.SubjectString, value: 8, code: "length"},
		{kind: validation.RuleMinItems, subject: validation.SubjectCollection, value: 2, code: "min_items"},
		{kind: validation.RuleMaxItems, subject: validation.SubjectCollection, value: 5, code: "max_items"},
		{kind: validation.RuleOneOf, subject: validation.SubjectString, value: 0, code: "one_of"},
		{kind: validation.RulePattern, subject: validation.SubjectString, value: 0, code: "pattern"},
		{kind: validation.RuleEmail, subject: validation.SubjectString, value: 0, code: "invalid_email"},
		{kind: validation.RuleURL, subject: validation.SubjectString, value: 0, code: "invalid_url"},
		{kind: validation.RuleUUID, subject: validation.SubjectString, value: 0, code: "invalid_uuid"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			p := validation.RuleProblem("/field", tt.kind, tt.subject, tt.value)
			if p.Code != tt.code {
				t.Errorf("got code %q, want %q", p.Code, tt.code)
			}
			if p.Message == "" {
				t.Error("message should not be empty")
			}
		})
	}
}
