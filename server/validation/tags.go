package validation

import (
	"reflect"
	"slices"
	"strings"
)

func FieldRequired(f reflect.StructField, tagKey string) bool {
	if required, ok := requiredOverride(f); ok {
		return required
	}
	rules, err := ParseFieldRules(f.Tag)
	if err == nil {
		if rules.Required {
			return true
		}
		if rules.OmitEmpty {
			return false
		}
	}
	return FieldRequiredFromTag(f.Tag.Get(tagKey), f.Type)
}

func FieldRequiredFromTag(tag string, typ reflect.Type) bool {
	if HasTagOption(tag, "omitempty") {
		return false
	}
	return typ.Kind() != reflect.Pointer
}

func HasTagOption(tag, option string) bool {
	if tag == "" {
		return false
	}
	parts := strings.Split(tag, ",")
	return slices.Contains(parts[1:], option)
}

func TagName(tag string) string {
	if tag == "" {
		return ""
	}
	return strings.Split(tag, ",")[0]
}

func JSONFieldName(f reflect.StructField) (string, bool) {
	tag := f.Tag.Get("json")
	if tag == "-" || tag == "" {
		return "", false
	}
	name := TagName(tag)
	if name == "" {
		name = f.Name
	}
	return name, true
}

func requiredOverride(f reflect.StructField) (bool, bool) {
	switch strings.ToLower(f.Tag.Get("required")) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}
