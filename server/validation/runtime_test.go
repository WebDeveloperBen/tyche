package validation_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/webdeveloperben/tyche/server/validation"
)

// validate builds the spec for v and runs ValidateStructValue against it.
func validate(t *testing.T, v any) error {
	t.Helper()
	spec, err := validation.Struct(reflect.TypeOf(v))
	if err != nil {
		t.Fatalf("Struct(%T): %v", v, err)
	}
	return validation.ValidateStructValue(reflect.ValueOf(v), spec, "request")
}

// problemCodes returns the error codes from a validation error.
func problemCodes(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var verr *validation.Error
	if !errors.As(err, &verr) {
		t.Fatalf("expected *validation.Error, got %T: %v", err, err)
	}
	codes := make([]string, len(verr.Problems))
	for i, p := range verr.Problems {
		codes[i] = p.Code
	}
	return codes
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func assertSingleCode(t *testing.T, err error, code string) {
	t.Helper()
	codes := problemCodes(t, err)
	if len(codes) != 1 || codes[0] != code {
		t.Fatalf("expected [%q], got %v", code, codes)
	}
}

// ── String rules ─────────────────────────────────────────────────────────────

func TestValidate_StringMin(t *testing.T) {
	type S struct {
		Name string `json:"name" validate:"min=3"`
	}
	assertSingleCode(t, validate(t, S{Name: "ab"}), "min") // below
	assertNoError(t, validate(t, S{Name: "abc"}))          // exact boundary
	assertNoError(t, validate(t, S{Name: "abcd"}))         // above
}

func TestValidate_StringMax(t *testing.T) {
	type S struct {
		Name string `json:"name" validate:"max=3"`
	}
	assertNoError(t, validate(t, S{Name: "abc"}))            // exact boundary
	assertSingleCode(t, validate(t, S{Name: "abcd"}), "max") // above
	assertNoError(t, validate(t, S{Name: "ab"}))             // below
}

func TestValidate_StringLen(t *testing.T) {
	type S struct {
		Code string `json:"code" validate:"len=4"`
	}
	assertNoError(t, validate(t, S{Code: "abcd"}))               // exact
	assertSingleCode(t, validate(t, S{Code: "abc"}), "length")   // short
	assertSingleCode(t, validate(t, S{Code: "abcde"}), "length") // long
}

func TestValidate_StringMin_Unicode(t *testing.T) {
	type S struct {
		Name string `json:"name" validate:"min=3"`
	}
	// Each emoji is 1 rune — "👍👍" is 2 runes, below min=3
	assertSingleCode(t, validate(t, S{Name: "👍👍"}), "min")
	// "👍👍👍" is 3 runes, at boundary
	assertNoError(t, validate(t, S{Name: "👍👍👍"}))
}

func TestValidate_StringLen_Unicode(t *testing.T) {
	type S struct {
		Code string `json:"code" validate:"len=2"`
	}
	// "中文" is 2 runes (6 bytes)
	assertNoError(t, validate(t, S{Code: "中文"}))
	// "中文字" is 3 runes
	assertSingleCode(t, validate(t, S{Code: "中文字"}), "length")
}

// ── Numeric rules ─────────────────────────────────────────────────────────────

func TestValidate_IntMin(t *testing.T) {
	type S struct {
		Age int `json:"age" validate:"min=18"`
	}
	assertSingleCode(t, validate(t, S{Age: 17}), "min")
	assertNoError(t, validate(t, S{Age: 18})) // boundary
	assertNoError(t, validate(t, S{Age: 99}))
}

func TestValidate_IntMax(t *testing.T) {
	type S struct {
		Score int `json:"score" validate:"max=100"`
	}
	assertNoError(t, validate(t, S{Score: 100})) // boundary
	assertSingleCode(t, validate(t, S{Score: 101}), "max")
}

func TestValidate_IntLen(t *testing.T) {
	type S struct {
		Count int `json:"count" validate:"len=5"`
	}
	assertNoError(t, validate(t, S{Count: 5}))
	assertSingleCode(t, validate(t, S{Count: 4}), "length")
	assertSingleCode(t, validate(t, S{Count: 6}), "length")
}

// ── Format rules ──────────────────────────────────────────────────────────────

func TestValidate_Email(t *testing.T) {
	type S struct {
		Email string `json:"email" validate:"email"`
	}
	assertNoError(t, validate(t, S{Email: "user@example.com"}))
	assertSingleCode(t, validate(t, S{Email: "not-an-email"}), "invalid_email")
	assertSingleCode(t, validate(t, S{Email: "missing@"}), "invalid_email")
	assertSingleCode(t, validate(t, S{Email: ""}), "invalid_email")
}

func TestValidate_URL(t *testing.T) {
	type S struct {
		Site string `json:"site" validate:"url"`
	}
	assertNoError(t, validate(t, S{Site: "https://example.com"}))
	assertNoError(t, validate(t, S{Site: "http://example.com/path?q=1"}))
	assertSingleCode(t, validate(t, S{Site: "not-a-url"}), "invalid_url")
	assertSingleCode(t, validate(t, S{Site: "//missing-scheme.com"}), "invalid_url")
	assertSingleCode(t, validate(t, S{Site: ""}), "invalid_url")
}

func TestValidate_UUID(t *testing.T) {
	type S struct {
		ID string `json:"id" validate:"uuid"`
	}
	assertNoError(t, validate(t, S{ID: "550e8400-e29b-41d4-a716-446655440000"}))
	assertNoError(t, validate(t, S{ID: "550E8400-E29B-41D4-A716-446655440000"})) // uppercase
	assertSingleCode(t, validate(t, S{ID: "not-a-uuid"}), "invalid_uuid")
	assertSingleCode(t, validate(t, S{ID: ""}), "invalid_uuid")
	assertSingleCode(t, validate(t, S{ID: "550e8400-e29b-41d4-a716-44665544000"}), "invalid_uuid") // too short
}

func TestValidate_Pattern(t *testing.T) {
	type S struct {
		Code string `json:"code" validate:"pattern=^[A-Z]{3}$"`
	}
	assertNoError(t, validate(t, S{Code: "ABC"}))
	assertSingleCode(t, validate(t, S{Code: "abc"}), "pattern")
	assertSingleCode(t, validate(t, S{Code: "AB"}), "pattern")
	assertSingleCode(t, validate(t, S{Code: ""}), "pattern")
}

func TestValidate_OneOf(t *testing.T) {
	type S struct {
		Role string `json:"role" validate:"oneof=admin member guest"`
	}
	assertNoError(t, validate(t, S{Role: "admin"}))
	assertNoError(t, validate(t, S{Role: "member"}))
	assertNoError(t, validate(t, S{Role: "guest"}))
	assertSingleCode(t, validate(t, S{Role: "superuser"}), "one_of")
	assertSingleCode(t, validate(t, S{Role: ""}), "one_of")
}

// ── Collection rules ──────────────────────────────────────────────────────────

func TestValidate_MinItems(t *testing.T) {
	type S struct {
		Tags []string `json:"tags" validate:"minItems=2"`
	}
	assertSingleCode(t, validate(t, S{Tags: []string{"a"}}), "min_items")
	assertNoError(t, validate(t, S{Tags: []string{"a", "b"}})) // boundary
	assertNoError(t, validate(t, S{Tags: []string{"a", "b", "c"}}))
}

func TestValidate_MaxItems(t *testing.T) {
	type S struct {
		Tags []string `json:"tags" validate:"maxItems=2"`
	}
	assertNoError(t, validate(t, S{Tags: []string{"a", "b"}})) // boundary
	assertSingleCode(t, validate(t, S{Tags: []string{"a", "b", "c"}}), "max_items")
}

// ── Items rules ───────────────────────────────────────────────────────────────

func TestValidate_ItemsEmail(t *testing.T) {
	type S struct {
		Emails []string `json:"emails" validate:"items.email"`
	}
	assertNoError(t, validate(t, S{Emails: []string{"a@b.com", "c@d.com"}}))
	err := validate(t, S{Emails: []string{"a@b.com", "not-email"}})
	codes := problemCodes(t, err)
	if len(codes) != 1 || codes[0] != "invalid_email" {
		t.Fatalf("expected [invalid_email], got %v", codes)
	}
}

func TestValidate_ItemsMin(t *testing.T) {
	type S struct {
		Codes []string `json:"codes" validate:"items.min=3"`
	}
	assertNoError(t, validate(t, S{Codes: []string{"abc", "def"}}))
	err := validate(t, S{Codes: []string{"abc", "de"}})
	assertSingleCode(t, err, "min")
}

func TestValidate_ItemsUUID(t *testing.T) {
	type S struct {
		IDs []string `json:"ids" validate:"items.uuid"`
	}
	assertNoError(t, validate(t, S{IDs: []string{"550e8400-e29b-41d4-a716-446655440000"}}))
	assertSingleCode(t, validate(t, S{IDs: []string{"not-a-uuid"}}), "invalid_uuid")
}

// ── OmitEmpty ─────────────────────────────────────────────────────────────────

func TestValidate_OmitEmpty_SkipsRulesOnZero(t *testing.T) {
	type S struct {
		Name string `json:"name" validate:"omitempty,min=5"`
	}
	// Empty string → omitempty skips the min rule
	assertNoError(t, validate(t, S{Name: ""}))
	// Non-empty string still validates
	assertSingleCode(t, validate(t, S{Name: "hi"}), "min")
	assertNoError(t, validate(t, S{Name: "hello"}))
}

// ── Required (param fields) ───────────────────────────────────────────────────

func TestValidate_QueryParamRequired(t *testing.T) {
	type S struct {
		Q string `query:"q" required:"true"`
	}
	// Zero value for a required param → required error
	assertSingleCode(t, validate(t, S{Q: ""}), "required")
	assertNoError(t, validate(t, S{Q: "search"}))
}

func TestValidate_PathParam_AlwaysRequired(t *testing.T) {
	type S struct {
		ID string `path:"id"`
	}
	assertSingleCode(t, validate(t, S{ID: ""}), "required")
	assertNoError(t, validate(t, S{ID: "123"}))
}

// ── Pointer fields ────────────────────────────────────────────────────────────

func TestValidate_NilPointer_Optional(t *testing.T) {
	type S struct {
		Name *string `json:"name,omitempty"`
	}
	// Nil pointer, no required tag → no error
	assertNoError(t, validate(t, S{Name: nil}))
}

func TestValidate_NilPointer_WithRules_Skipped(t *testing.T) {
	type S struct {
		Name *string `json:"name,omitempty" validate:"min=3"`
	}
	// Nil pointer → rules are not applied
	assertNoError(t, validate(t, S{Name: nil}))

	// Non-nil pointer that fails the rule
	short := "ab"
	assertSingleCode(t, validate(t, S{Name: &short}), "min")

	// Non-nil pointer that passes
	long := "abc"
	assertNoError(t, validate(t, S{Name: &long}))
}

// ── Nested structs ────────────────────────────────────────────────────────────

func TestValidate_NestedStruct(t *testing.T) {
	type Address struct {
		City string `json:"city" validate:"min=2"`
	}
	type S struct {
		Addr Address `json:"addr"`
	}
	assertSingleCode(t, validate(t, S{Addr: Address{City: "X"}}), "min")
	assertNoError(t, validate(t, S{Addr: Address{City: "NY"}}))
}

func TestValidate_SliceOfStructs(t *testing.T) {
	type Item struct {
		Code string `json:"code" validate:"min=3"`
	}
	type S struct {
		Items []Item `json:"items"`
	}
	err := validate(t, S{Items: []Item{{Code: "AB"}, {Code: "CDE"}}})
	// First item fails, second passes → 1 error
	assertSingleCode(t, err, "min")
}

func TestValidate_MultipleErrors(t *testing.T) {
	type S struct {
		Name  string `json:"name" validate:"min=2"`
		Email string `json:"email" validate:"email"`
	}
	err := validate(t, S{Name: "X", Email: "bad"})
	codes := problemCodes(t, err)
	if len(codes) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(codes), codes)
	}
}

// ── ValidateUUID ──────────────────────────────────────────────────────────────

func TestValidateUUID(t *testing.T) {
	valid := []string{
		"550e8400-e29b-41d4-a716-446655440000",
		"00000000-0000-0000-0000-000000000000",
		"FFFFFFFF-FFFF-FFFF-FFFF-FFFFFFFFFFFF",
		"ffffffff-ffff-ffff-ffff-ffffffffffff",
	}
	for _, v := range valid {
		if !validation.ValidateUUID(v) {
			t.Errorf("expected valid UUID: %q", v)
		}
	}

	invalid := []string{
		"",
		"not-a-uuid",
		"550e8400-e29b-41d4-a716-44665544000",   // too short
		"550e8400-e29b-41d4-a716-4466554400000", // too long
		"550e8400xe29b-41d4-a716-446655440000",  // wrong separator at pos 8
		"550e8400-e29b-41d4-a716-44665544000g",  // invalid hex char
		"550e840g-e29b-41d4-a716-446655440000",  // invalid hex in first group
	}
	for _, v := range invalid {
		if validation.ValidateUUID(v) {
			t.Errorf("expected invalid UUID: %q", v)
		}
	}
}

// ── StringLength ──────────────────────────────────────────────────────────────

func TestStringLength(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abc", 3},
		{"hello world", 11},
		{"👍", 1},            // 4 bytes, 1 rune
		{"👍👍👍", 3},          // 12 bytes, 3 runes
		{"中文", 2},           // 6 bytes, 2 runes
		{"hello👍world", 11}, // mixed
	}
	for _, tt := range tests {
		got := validation.StringLength(tt.input)
		if got != tt.want {
			t.Errorf("StringLength(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}
