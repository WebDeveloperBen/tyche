package validation

import (
	"mime/multipart"
	"reflect"
	"slices"
	"strings"
)

// FieldRequired reports whether a bound field must be present in the request.
// Query, header, and cookie parameters are optional unless marked
// required:"true" or validate:"required" (matching OpenAPI's default for
// non-path parameters). Body, form, and file fields are required unless the
// field is a pointer or opts out via omitempty / required:"false".
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
	if isOptionalParamSource(tagKey) {
		return false
	}
	if tagKey == "file" && isMultipartFileHeader(f.Type) {
		return !HasTagOption(f.Tag.Get(tagKey), "omitempty")
	}
	return FieldRequiredFromTag(f.Tag.Get(tagKey), f.Type)
}

func isOptionalParamSource(tagKey string) bool {
	return tagKey == "query" || tagKey == "header" || tagKey == "cookie"
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

func isMultipartFileHeader(t reflect.Type) bool {
	return t == reflect.TypeFor[*multipart.FileHeader]()
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
