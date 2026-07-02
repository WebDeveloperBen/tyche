package validation_test

import (
	"reflect"
	"testing"

	"github.com/webdeveloperben/tyche/server/validation"
)

// TestConstraintsForField_EnumTyping verifies oneof enum values are coerced to
// match the schema type, so the emitted OpenAPI enum is consistent with the
// field's type (integer -> [1,2,3], not ["1","2","3"]).
func TestConstraintsForField_EnumTyping(t *testing.T) {
	type S struct {
		Priority int     `validate:"oneof=1 2 3"`
		Ratio    float64 `validate:"oneof=1.5 2.5"`
		Kind     string  `validate:"oneof=a b"`
	}

	field := func(name string) reflect.StructField {
		f, ok := reflect.TypeFor[S]().FieldByName(name)
		if !ok {
			t.Fatalf("no field %s", name)
		}
		return f
	}

	cases := []struct {
		field      string
		schemaType string
		want       []any
	}{
		{"Priority", "integer", []any{int64(1), int64(2), int64(3)}},
		{"Ratio", "number", []any{1.5, 2.5}},
		{"Kind", "string", []any{"a", "b"}},
	}
	for _, tc := range cases {
		c, err := validation.ConstraintsForField(field(tc.field), tc.schemaType)
		if err != nil {
			t.Fatalf("%s: %v", tc.field, err)
		}
		if !reflect.DeepEqual(c.Enum, tc.want) {
			t.Fatalf("%s (%s): enum = %#v, want %#v", tc.field, tc.schemaType, c.Enum, tc.want)
		}
	}
}
