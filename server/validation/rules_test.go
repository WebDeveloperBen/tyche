package validation_test

import (
	"reflect"
	"testing"

	"github.com/webdeveloperben/tyche/server/validation"
)

func TestParseFieldRules_Empty(t *testing.T) {
	rules, err := validation.ParseFieldRules(reflect.StructTag(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.Rules) != 0 || len(rules.ItemRules) != 0 || rules.Required || rules.OmitEmpty {
		t.Errorf("expected zero-value rules for empty tag, got %+v", rules)
	}
}

func TestParseFieldRules_BooleanRules(t *testing.T) {
	t.Run("required", func(t *testing.T) {
		rules, err := validation.ParseFieldRules(reflect.StructTag(`validate:"required"`))
		if err != nil {
			t.Fatal(err)
		}
		if !rules.Required {
			t.Error("expected Required=true")
		}
	})

	t.Run("omitempty", func(t *testing.T) {
		rules, err := validation.ParseFieldRules(reflect.StructTag(`validate:"omitempty"`))
		if err != nil {
			t.Fatal(err)
		}
		if !rules.OmitEmpty {
			t.Error("expected OmitEmpty=true")
		}
	})

	t.Run("required and omitempty conflict", func(t *testing.T) {
		_, err := validation.ParseFieldRules(reflect.StructTag(`validate:"required,omitempty"`))
		if err == nil {
			t.Fatal("expected error for required+omitempty conflict")
		}
	})
}

func TestParseFieldRules_StringFormatRules(t *testing.T) {
	tests := []struct {
		tag  string
		kind validation.RuleKind
	}{
		{`validate:"email"`, validation.RuleEmail},
		{`validate:"url"`, validation.RuleURL},
		{`validate:"uuid"`, validation.RuleUUID},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			rules, err := validation.ParseFieldRules(reflect.StructTag(tt.tag))
			if err != nil {
				t.Fatal(err)
			}
			if len(rules.Rules) != 1 || rules.Rules[0].Kind != tt.kind {
				t.Errorf("unexpected rules: %+v", rules.Rules)
			}
		})
	}
}

func TestParseFieldRules_IntRules(t *testing.T) {
	tests := []struct {
		tag     string
		kind    validation.RuleKind
		wantInt int
	}{
		{`validate:"min=3"`, validation.RuleMin, 3},
		{`validate:"max=10"`, validation.RuleMax, 10},
		{`validate:"len=5"`, validation.RuleLen, 5},
		{`validate:"minItems=1"`, validation.RuleMinItems, 1},
		{`validate:"maxItems=100"`, validation.RuleMaxItems, 100},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			rules, err := validation.ParseFieldRules(reflect.StructTag(tt.tag))
			if err != nil {
				t.Fatal(err)
			}
			if len(rules.Rules) != 1 {
				t.Fatalf("expected 1 rule, got %d", len(rules.Rules))
			}
			r := rules.Rules[0]
			if r.Kind != tt.kind || r.Int != tt.wantInt {
				t.Errorf("got kind=%q int=%d, want kind=%q int=%d", r.Kind, r.Int, tt.kind, tt.wantInt)
			}
		})
	}
}

func TestParseFieldRules_MultipleRules(t *testing.T) {
	rules, err := validation.ParseFieldRules(reflect.StructTag(`validate:"min=2,max=50,email"`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d: %+v", len(rules.Rules), rules.Rules)
	}
	if rules.Rules[0].Kind != validation.RuleMin || rules.Rules[0].Int != 2 {
		t.Errorf("first rule: %+v", rules.Rules[0])
	}
	if rules.Rules[1].Kind != validation.RuleMax || rules.Rules[1].Int != 50 {
		t.Errorf("second rule: %+v", rules.Rules[1])
	}
	if rules.Rules[2].Kind != validation.RuleEmail {
		t.Errorf("third rule: %+v", rules.Rules[2])
	}
}

func TestParseFieldRules_OneOf(t *testing.T) {
	rules, err := validation.ParseFieldRules(reflect.StructTag(`validate:"oneof=admin member guest"`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.Rules) != 1 || rules.Rules[0].Kind != validation.RuleOneOf {
		t.Fatalf("unexpected rules: %+v", rules)
	}
	list := rules.Rules[0].List
	if len(list) != 3 || list[0] != "admin" || list[1] != "member" || list[2] != "guest" {
		t.Errorf("unexpected oneof list: %v", list)
	}
}

func TestParseFieldRules_Pattern(t *testing.T) {
	rules, err := validation.ParseFieldRules(reflect.StructTag(`validate:"pattern=^[a-z]+$"`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rules.Rules) != 1 || rules.Rules[0].Kind != validation.RulePattern {
		t.Fatalf("unexpected rules: %+v", rules)
	}
	if rules.Rules[0].String != "^[a-z]+$" {
		t.Errorf("got pattern %q, want %q", rules.Rules[0].String, "^[a-z]+$")
	}
}

func TestParseFieldRules_ItemsRules(t *testing.T) {
	tests := []struct {
		tag      string
		wantKind validation.RuleKind
	}{
		{`validate:"items.email"`, validation.RuleEmail},
		{`validate:"items.url"`, validation.RuleURL},
		{`validate:"items.uuid"`, validation.RuleUUID},
		{`validate:"items.min=3"`, validation.RuleMin},
		{`validate:"items.max=10"`, validation.RuleMax},
	}
	for _, tt := range tests {
		t.Run(string(tt.wantKind), func(t *testing.T) {
			rules, err := validation.ParseFieldRules(reflect.StructTag(tt.tag))
			if err != nil {
				t.Fatal(err)
			}
			if len(rules.ItemRules) != 1 || rules.ItemRules[0].Kind != tt.wantKind {
				t.Errorf("unexpected item rules: %+v", rules.ItemRules)
			}
		})
	}
}

func TestParseFieldRules_Errors(t *testing.T) {
	tests := []struct {
		name string
		tag  string
	}{
		{"unknown rule", `validate:"bogus"`},
		{"invalid min value", `validate:"min=abc"`},
		{"invalid max value", `validate:"max=xyz"`},
		{"invalid len value", `validate:"len=one"`},
		{"empty pattern", `validate:"pattern="`},
		{"empty oneof", `validate:"oneof="`},
		{"unknown items rule", `validate:"items.bogus"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validation.ParseFieldRules(reflect.StructTag(tt.tag))
			if err == nil {
				t.Fatalf("expected error for tag %q", tt.tag)
			}
		})
	}
}

func TestStruct_RuleCompatibilityErrors(t *testing.T) {
	t.Run("min on bool field", func(t *testing.T) {
		type S struct {
			Active bool `json:"active" validate:"min=1"`
		}
		_, err := validation.Struct(reflect.TypeFor[S]())
		if err == nil {
			t.Fatal("expected error for min on bool field")
		}
	})

	t.Run("minItems on string field", func(t *testing.T) {
		type S struct {
			Name string `json:"name" validate:"minItems=1"`
		}
		_, err := validation.Struct(reflect.TypeFor[S]())
		if err == nil {
			t.Fatal("expected error for minItems on string field")
		}
	})

	t.Run("email on int field", func(t *testing.T) {
		type S struct {
			Age int `json:"age" validate:"email"`
		}
		_, err := validation.Struct(reflect.TypeFor[S]())
		if err == nil {
			t.Fatal("expected error for email on int field")
		}
	})
}
