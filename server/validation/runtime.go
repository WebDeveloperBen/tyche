package validation

import (
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"sync"
	"unicode/utf8"
)

var regexCache sync.Map

func ValidateStructValue(v reflect.Value, spec *StructSpec, scope string) error {
	var errs Error
	validateStructValue(&errs, v, spec, scope)
	if errs.Empty() {
		return nil
	}
	return &errs
}

func validateStructValue(errs *Error, v reflect.Value, spec *StructSpec, scope string) {
	if !v.IsValid() {
		return
	}
	if v.Kind() != reflect.Struct || isScalarStruct(v.Type()) {
		return
	}

	for _, field := range spec.Fields {
		fieldValue := v.Field(field.Index)
		var fieldScope string
		if field.FullPointer != "" {
			fieldScope = field.FullPointer
		} else {
			fieldScope = scope
		}
		validateFieldValue(errs, fieldValue, field, fieldScope)
		if field.Nested != nil {
			validateStructValue(errs, fieldValue, field.Nested, fieldScope)
		}
		validateCollectionElements(errs, fieldValue, field, scope)
	}
}

func validateFieldValue(errs *Error, v reflect.Value, field FieldSpec, scope string) {
	pointer := scope
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			if field.Required && !field.HasParam {
				errs.AddRequired(pointer)
			}
			return
		}
		validateFieldValue(errs, v.Elem(), field, scope)
		return
	}

	if field.Required && field.HasParam && isZeroValue(v) {
		errs.AddRequired(pointer)
	}
	if field.Rules.OmitEmpty && isZeroValue(v) {
		return
	}

	for _, rule := range field.Rules.Rules {
		applyRule(errs, v, pointer, rule)
	}
}

func validateCollectionElements(errs *Error, v reflect.Value, field FieldSpec, scope string) {
	if field.ElemNested == nil && len(field.Rules.ItemRules) == 0 {
		return
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Slice, reflect.Array:
	default:
		return
	}
	for i := 0; i < v.Len(); i++ {
		itemValue := v.Index(i)
		itemPointer := JoinPointerWithIndex(scope, i)
		if field.ElemNested != nil {
			validateStructValue(errs, itemValue, field.ElemNested, itemPointer)
		}
		for _, rule := range field.Rules.ItemRules {
			applyRule(errs, itemValue, itemPointer, rule)
		}
	}
}

func applyRule(errs *Error, v reflect.Value, pointer string, rule Rule) {
	switch rule.Kind {
	case RuleMin:
		switch v.Kind() {
		case reflect.String:
			if StringLength(v.String()) < rule.Int {
				errs.AddRule(pointer, RuleMin, SubjectString, rule.Int)
			}
		default:
			if numericValue(v) < float64(rule.Int) {
				errs.AddRule(pointer, RuleMin, SubjectNumber, rule.Int)
			}
		}
	case RuleMax:
		switch v.Kind() {
		case reflect.String:
			if StringLength(v.String()) > rule.Int {
				errs.AddRule(pointer, RuleMax, SubjectString, rule.Int)
			}
		default:
			if numericValue(v) > float64(rule.Int) {
				errs.AddRule(pointer, RuleMax, SubjectNumber, rule.Int)
			}
		}
	case RuleLen:
		switch v.Kind() {
		case reflect.String:
			if StringLength(v.String()) != rule.Int {
				errs.AddRule(pointer, RuleLen, SubjectString, rule.Int)
			}
		default:
			if numericValue(v) != float64(rule.Int) {
				errs.AddRule(pointer, RuleLen, SubjectNumber, rule.Int)
			}
		}
	case RuleMinItems:
		if v.Len() < rule.Int {
			errs.AddRule(pointer, RuleMinItems, SubjectCollection, rule.Int)
		}
	case RuleMaxItems:
		if v.Len() > rule.Int {
			errs.AddRule(pointer, RuleMaxItems, SubjectCollection, rule.Int)
		}
	case RuleOneOf:
		if !slices.Contains(rule.List, v.String()) {
			errs.AddRule(pointer, RuleOneOf, SubjectString, 0)
		}
	case RulePattern:
		re, err := compiledPattern(rule.String)
		if err != nil {
			errs.AddInvalidRule(pointer, err)
			return
		}
		if !re.MatchString(v.String()) {
			errs.AddRule(pointer, RulePattern, SubjectString, 0)
		}
	case RuleEmail:
		if _, err := mail.ParseAddress(v.String()); err != nil {
			errs.AddRule(pointer, RuleEmail, SubjectString, 0)
		}
	case RuleURL:
		parsed, err := url.ParseRequestURI(v.String())
		if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
			errs.AddRule(pointer, RuleURL, SubjectString, 0)
		}
	case RuleUUID:
		if !ValidateUUID(v.String()) {
			errs.AddRule(pointer, RuleUUID, SubjectString, 0)
		}
	}
}

func compiledPattern(pattern string) (*regexp.Regexp, error) {
	if cached, ok := regexCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	actual, _ := regexCache.LoadOrStore(pattern, re)
	return actual.(*regexp.Regexp), nil
}

func numericValue(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	default:
		return 0
	}
}

func ValidateUUID(value string) bool {
	switch len(value) {
	case 36:
		return validateHyphenatedUUID(value)
	case 32:
		return validateHexOnlyUUID(value)
	default:
		return false
	}
}

func validateHyphenatedUUID(value string) bool {
	for i := range value {
		switch i {
		case 8, 13, 18, 23:
			if value[i] != '-' {
				return false
			}
		default:
			if !isHexByte(value[i]) {
				return false
			}
		}
	}
	return true
}

func validateHexOnlyUUID(value string) bool {
	for i := range value {
		if !isHexByte(value[i]) {
			return false
		}
	}
	return true
}

func StringLength(value string) int {
	return utf8.RuneCountInString(value)
}

func isHexByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func isZeroValue(v reflect.Value) bool {
	return !v.IsValid() || v.IsZero()
}
