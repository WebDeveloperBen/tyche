package validation_test

import (
	"reflect"
	"testing"

	"github.com/webdeveloperben/tyche/server/validation"
)

func TestTagName(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"", ""},
		{"name", "name"},
		{"name,omitempty", "name"},
		{"my-field,omitempty,extra", "my-field"},
		{",omitempty", ""},
	}
	for _, tt := range tests {
		got := validation.TagName(tt.tag)
		if got != tt.want {
			t.Errorf("TagName(%q) = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

func TestJSONFieldName(t *testing.T) {
	type S struct {
		Named     string `json:"named"`
		OmitEmpty string `json:"omit,omitempty"`
		Ignored   string `json:"-"`
		NoTag     string
		EmptyName string `json:",omitempty"` // name falls back to field name
	}
	typ := reflect.TypeFor[S]()

	tests := []struct {
		field    string
		wantName string
		wantOK   bool
	}{
		{"Named", "named", true},
		{"OmitEmpty", "omit", true},
		{"Ignored", "", false},
		{"NoTag", "", false},
		{"EmptyName", "EmptyName", true}, // empty json name → field name
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f, _ := typ.FieldByName(tt.field)
			name, ok := validation.JSONFieldName(f)
			if ok != tt.wantOK || name != tt.wantName {
				t.Errorf("got (%q, %v), want (%q, %v)", name, ok, tt.wantName, tt.wantOK)
			}
		})
	}
}

func TestHasTagOption(t *testing.T) {
	tests := []struct {
		tag    string
		option string
		want   bool
	}{
		{"", "omitempty", false},
		{"name", "omitempty", false},
		{"name,omitempty", "omitempty", true},
		{"name,omitempty,foo", "omitempty", true},
		{"name,omitempty,foo", "foo", true},
		{"name,foo", "omitempty", false},
		// option must not match the name segment
		{"omitempty", "omitempty", false},
	}
	for _, tt := range tests {
		got := validation.HasTagOption(tt.tag, tt.option)
		if got != tt.want {
			t.Errorf("HasTagOption(%q, %q) = %v, want %v", tt.tag, tt.option, got, tt.want)
		}
	}
}

func TestFieldRequired(t *testing.T) {
	type S struct {
		QueryOpt  *string `query:"page"`
		JSONOpt   *string `json:"bio"`
		ExplTrue  *string `json:"et" required:"true"`
		PathID    string  `path:"id"`
		QueryReq  string  `query:"q" required:"true"`
		QueryOmit string  `query:"limit,omitempty"`
		JSONReq   string  `json:"name"`
		JSONOmit  string  `json:"role,omitempty"`
		ValidOmit string  `json:"tag" validate:"omitempty"`
		ExplFalse string  `json:"ef" required:"false"`
	}
	typ := reflect.TypeFor[S]()

	tests := []struct {
		field  string
		tagKey string
		want   bool
	}{
		{"PathID", "path", true},
		{"QueryReq", "query", true},
		{"QueryOpt", "query", false},  // pointer
		{"QueryOmit", "query", false}, // omitempty option
		{"JSONReq", "json", true},
		{"JSONOpt", "json", false},   // pointer
		{"JSONOmit", "json", false},  // omitempty option
		{"ValidOmit", "json", false}, // validate:"omitempty"
		{"ExplFalse", "json", false}, // required:"false"
		{"ExplTrue", "json", true},   // required:"true" overrides pointer
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			f, ok := typ.FieldByName(tt.field)
			if !ok {
				t.Fatalf("field %s not found", tt.field)
			}
			got := validation.FieldRequired(f, tt.tagKey)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFieldRequiredFromTag(t *testing.T) {
	strType := reflect.TypeFor[string]()
	ptrType := reflect.TypeFor[*string]()

	tests := []struct {
		typ  reflect.Type
		tag  string
		want bool
	}{
		{tag: "name", typ: strType, want: true},            // non-pointer, no omitempty
		{tag: "name,omitempty", typ: strType, want: false}, // omitempty present
		{tag: "name", typ: ptrType, want: false},           // pointer type
		{tag: "", typ: strType, want: true},                // no tag, non-pointer
	}
	for _, tt := range tests {
		got := validation.FieldRequiredFromTag(tt.tag, tt.typ)
		if got != tt.want {
			t.Errorf("FieldRequiredFromTag(%q, %v) = %v, want %v", tt.tag, tt.typ, got, tt.want)
		}
	}
}
