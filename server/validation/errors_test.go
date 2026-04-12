package validation_test

import (
	"testing"

	"github.com/webdeveloperben/tyche/server/validation"
)

func TestJSONPointer(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{"nil parts", nil, ""},
		{"single empty part", []string{""}, ""},
		{"single part", []string{"foo"}, "/foo"},
		{"two parts", []string{"foo", "bar"}, "/foo/bar"},
		{"three parts", []string{"a", "b", "c"}, "/a/b/c"},
		{"tilde escape", []string{"tilde~field"}, "/tilde~0field"},
		{"slash escape", []string{"slash/field"}, "/slash~1field"},
		{"both escapes", []string{"a~b/c"}, "/a~0b~1c"},
		{"skip empty parts", []string{"a", "", "b"}, "/a/b"},
		{"leading empty part", []string{"", "a"}, "/a"},
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
		parts []string
		want  string
	}{
		{"empty base uses JSONPointer", "", []string{"foo"}, "/foo"},
		{"base with part", "/base", []string{"field"}, "/base/field"},
		{"bare base with part", "base", []string{"field"}, "base/field"},
		{"skip empty parts", "/base", []string{"", "field"}, "/base/field"},
		{"multiple parts", "/root", []string{"a", "b"}, "/root/a/b"},
		{"no parts", "/base", nil, "/base"},
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
		index int
		want  string
	}{
		{"empty base", "", 0, "0"},
		{"empty base non-zero index", "", 5, "5"},
		{"with base", "/items", 0, "/items/0"},
		{"with base non-zero", "/items", 3, "/items/3"},
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
		value   int
		code    string
	}{
		{validation.RuleMin, validation.SubjectString, 5, "min"},
		{validation.RuleMin, validation.SubjectNumber, 0, "min"},
		{validation.RuleMax, validation.SubjectString, 10, "max"},
		{validation.RuleMax, validation.SubjectNumber, 100, "max"},
		{validation.RuleLen, validation.SubjectString, 8, "length"},
		{validation.RuleMinItems, validation.SubjectCollection, 2, "min_items"},
		{validation.RuleMaxItems, validation.SubjectCollection, 5, "max_items"},
		{validation.RuleOneOf, validation.SubjectString, 0, "one_of"},
		{validation.RulePattern, validation.SubjectString, 0, "pattern"},
		{validation.RuleEmail, validation.SubjectString, 0, "invalid_email"},
		{validation.RuleURL, validation.SubjectString, 0, "invalid_url"},
		{validation.RuleUUID, validation.SubjectString, 0, "invalid_uuid"},
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
