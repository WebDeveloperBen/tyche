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

	fieldCount := v.NumField()
	for i := range spec.Fields {
		field := &spec.Fields[i]
		var fieldValue reflect.Value
		if len(field.IndexPath) == 1 && field.IndexPath[0] < fieldCount {
			// The common case is a direct field. Avoid the pointer/struct
			// walk needed for flattened embedded fields.
			fieldValue = v.Field(field.IndexPath[0])
		} else {
			var ok bool
			fieldValue, ok = fieldValueByIndex(v, field.IndexPath)
			if !ok {
				continue
			}
		}
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

func validateFieldValue(errs *Error, v reflect.Value, field *FieldSpec, scope string) {
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

func validateCollectionElements(errs *Error, v reflect.Value, field *FieldSpec, scope string) {
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
	for i := range v.Len() {
		itemValue := v.Index(i)
		itemPointer := ""
		if field.ElemNested != nil {
			itemPointer = JoinPointerWithIndex(scope, i)
			validateStructValue(errs, itemValue, field.ElemNested, itemPointer)
		}
		for _, rule := range field.Rules.ItemRules {
			applyItemRule(errs, itemValue, scope, i, itemPointer, rule)
		}
	}
}

type ruleFailure struct {
	kind    RuleKind
	subject Subject
	value   int
	err     error
	invalid bool
}

func applyRule(errs *Error, v reflect.Value, pointer string, rule Rule) {
	failure := evaluateRule(v, rule)
	if failure.err != nil {
		errs.AddInvalidRule(pointer, failure.err)
	} else if failure.invalid {
		errs.AddRule(pointer, failure.kind, failure.subject, failure.value)
	}
}

func applyItemRule(errs *Error, v reflect.Value, scope string, index int, pointer string, rule Rule) {
	failure := evaluateRule(v, rule)
	if !failure.invalid && failure.err == nil {
		return
	}
	if pointer == "" {
		pointer = JoinPointerWithIndex(scope, index)
	}
	if failure.err != nil {
		errs.AddInvalidRule(pointer, failure.err)
	} else {
		errs.AddRule(pointer, failure.kind, failure.subject, failure.value)
	}
}

func evaluateRule(v reflect.Value, rule Rule) ruleFailure {
	switch rule.Kind {
	case RuleMin:
		switch v.Kind() {
		case reflect.String:
			if StringLength(v.String()) < rule.Int {
				return ruleFailure{kind: RuleMin, subject: SubjectString, value: rule.Int, invalid: true}
			}
		default:
			if numericValue(v) < float64(rule.Int) {
				return ruleFailure{kind: RuleMin, subject: SubjectNumber, value: rule.Int, invalid: true}
			}
		}
	case RuleMax:
		switch v.Kind() {
		case reflect.String:
			if StringLength(v.String()) > rule.Int {
				return ruleFailure{kind: RuleMax, subject: SubjectString, value: rule.Int, invalid: true}
			}
		default:
			if numericValue(v) > float64(rule.Int) {
				return ruleFailure{kind: RuleMax, subject: SubjectNumber, value: rule.Int, invalid: true}
			}
		}
	case RuleLen:
		switch v.Kind() {
		case reflect.String:
			if StringLength(v.String()) != rule.Int {
				return ruleFailure{kind: RuleLen, subject: SubjectString, value: rule.Int, invalid: true}
			}
		default:
			if numericValue(v) != float64(rule.Int) {
				return ruleFailure{kind: RuleLen, subject: SubjectNumber, value: rule.Int, invalid: true}
			}
		}
	case RuleMinItems:
		if v.Len() < rule.Int {
			return ruleFailure{kind: RuleMinItems, subject: SubjectCollection, value: rule.Int, invalid: true}
		}
	case RuleMaxItems:
		if v.Len() > rule.Int {
			return ruleFailure{kind: RuleMaxItems, subject: SubjectCollection, value: rule.Int, invalid: true}
		}
	case RuleOneOf:
		if !slices.Contains(rule.List, v.String()) {
			return ruleFailure{kind: RuleOneOf, subject: SubjectString, invalid: true}
		}
	case RulePattern:
		re, err := compiledPattern(rule.String)
		if err != nil {
			return ruleFailure{err: err}
		}
		if !re.MatchString(v.String()) {
			return ruleFailure{kind: RulePattern, subject: SubjectString, invalid: true}
		}
	case RuleEmail:
		if _, err := mail.ParseAddress(v.String()); err != nil {
			return ruleFailure{kind: RuleEmail, subject: SubjectString, invalid: true}
		}
	case RuleURL:
		parsed, err := url.ParseRequestURI(v.String())
		if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
			return ruleFailure{kind: RuleURL, subject: SubjectString, invalid: true}
		}
	case RuleUUID:
		if !ValidateUUID(v.String()) {
			return ruleFailure{kind: RuleUUID, subject: SubjectString, invalid: true}
		}
	}
	return ruleFailure{}
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

func fieldValueByIndex(value reflect.Value, index []int) (reflect.Value, bool) {
	for _, position := range index {
		for value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		if value.Kind() != reflect.Struct || position >= value.NumField() {
			return reflect.Value{}, false
		}
		value = value.Field(position)
	}
	return value, value.IsValid()
}

func isHexByte(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

func isZeroValue(v reflect.Value) bool {
	return !v.IsValid() || v.IsZero()
}
