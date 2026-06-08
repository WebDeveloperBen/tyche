package servergen

import (
	"go/types"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/webdeveloperben/tyche/server/validation"
)

func tagName(tag string) string {
	if tag == "" {
		return ""
	}
	return strings.Split(tag, ",")[0]
}

func analyseInputType(t types.Type) InputBindSpec {
	spec := InputBindSpec{Manual: true}
	named, ok := t.(*types.Named)
	if ok {
		t = named.Underlying()
	}
	strct, ok := t.(*types.Struct)
	if !ok {
		return InputBindSpec{}
	}

	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if !field.Exported() {
			continue
		}
		tag := reflect.StructTag(strct.Tag(i))
		switch {
		case tag.Get("body") != "" || field.Name() == "Body":
			bodySpec, ok := analyseBodyStruct(field.Type(), "in."+field.Name()+".", fieldRequiredForJSONTag(tag, field.Type()))
			if !ok {
				bodySpec, ok = analyseDirectBodyType(field.Type(), "in."+field.Name(), fieldRequiredForJSONTag(tag, field.Type()), tag)
				if !ok {
					return InputBindSpec{}
				}
			}
			bodySpec.DecodeTarget = "in." + field.Name()
			spec.Body = bodySpec
		case tag.Get("json") != "":
			bodySpec, ok := analyseBodyStruct(t, "in.", hasRequiredJSONFieldsForTypesStruct(strct))
			if !ok {
				return InputBindSpec{}
			}
			spec.Body = bodySpec
			spec.Body.Target = "in."
			spec.Body.DecodeTarget = "in"
			return spec
		}

		source, name := fieldSource(tag)
		if source == "" {
			continue
		}

		typeExpr, kind, pointer, ok := supportedScalar(field.Type())
		if !ok {
			return InputBindSpec{}
		}

		spec.Fields = append(spec.Fields, BindFieldSpec{
			FieldName: field.Name(),
			ParamName: name,
			Source:    source,
			TypeExpr:  typeExpr,
			Kind:      kind,
			Pointer:   pointer,
			Required:  requiredForTag(tag, source, field.Type()),
			Rules:     mustParseRules(tag),
		})
	}

	return spec
}

func analyseDirectBodyType(t types.Type, target string, required bool, tag reflect.StructTag) (*BodyBindSpec, bool) {
	field := BodyFieldSpec{
		TypeExpr: types.TypeString(t, nil),
		Required: required,
		Opaque:   true,
	}
	if typeExpr, kind, pointer, ok := supportedScalar(t); ok {
		field.TypeExpr = typeExpr
		field.Kind = kind
		field.Pointer = pointer
		field.Opaque = false
		if !applyBodyFieldValidation(&field, tag) {
			return nil, false
		}
	} else if sliceElem, ok := sliceElementType(t); ok {
		field.Slice = true
		if elemTypeExpr, elemKind, elemPtr, ok := supportedScalar(sliceElem); ok {
			field.ElemType = elemTypeExpr
			field.ElemKind = elemKind
			field.ElemPtr = elemPtr
			field.Opaque = false
			if !applyBodyFieldValidation(&field, tag) {
				return nil, false
			}
		} else if nestedType, nestedPtr, ok := nestedStructType(sliceElem); ok {
			nested, ok := analyseBodyStruct(nestedType, "", required)
			if !ok {
				return nil, false
			}
			field.ElemNested = nested
			field.ElemStruct = types.TypeString(nestedType, nil)
			field.ElemStructPtr = nestedPtr
			field.Opaque = false
		}
	}

	return &BodyBindSpec{
		Required:     required,
		Target:       target,
		DecodeTarget: target,
		Direct:       &field,
	}, true
}

func analyseBodyStruct(t types.Type, targetPrefix string, required bool) (*BodyBindSpec, bool) {
	underlying := t
	if named, ok := t.(*types.Named); ok {
		underlying = named.Underlying()
	}
	strct, ok := underlying.(*types.Struct)
	if !ok {
		return nil, false
	}

	body := &BodyBindSpec{Required: required, Target: targetPrefix}
	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if !field.Exported() {
			continue
		}
		tag := reflect.StructTag(strct.Tag(i))
		if fieldSourceName, _ := fieldSource(tag); fieldSourceName != "" {
			return nil, false
		}
		jsonTag := tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		bodyField := BodyFieldSpec{
			FieldName: field.Name(),
			JSONName:  tagName(jsonTag),
			Required:  fieldRequiredForJSONTag(tag, field.Type()),
		}
		if bodyField.JSONName == "" {
			bodyField.JSONName = field.Name()
		}

		if typeExpr, kind, pointer, ok := supportedScalar(field.Type()); ok {
			bodyField.TypeExpr = typeExpr
			bodyField.Kind = kind
			bodyField.Pointer = pointer
			if !applyBodyFieldValidation(&bodyField, tag) {
				return nil, false
			}
			body.Fields = append(body.Fields, bodyField)
			continue
		}

		if sliceElem, ok := sliceElementType(field.Type()); ok {
			bodyField.Slice = true
			if elemTypeExpr, elemKind, elemPtr, ok := supportedScalar(sliceElem); ok {
				bodyField.ElemType = elemTypeExpr
				bodyField.ElemKind = elemKind
				bodyField.ElemPtr = elemPtr
				if !applyBodyFieldValidation(&bodyField, tag) {
					return nil, false
				}
				body.Fields = append(body.Fields, bodyField)
				continue
			}
			nestedType, nestedPtr, ok := nestedStructType(sliceElem)
			if !ok {
				bodyField.TypeExpr = types.TypeString(field.Type(), nil)
				bodyField.Opaque = true
				body.Fields = append(body.Fields, bodyField)
				continue
			}
			nested, ok := analyseBodyStruct(nestedType, "", bodyField.Required)
			if !ok {
				return nil, false
			}
			bodyField.ElemNested = nested
			bodyField.ElemStruct = types.TypeString(nestedType, nil)
			bodyField.ElemStructPtr = nestedPtr
			body.Fields = append(body.Fields, bodyField)
			continue
		}

		nestedType, nestedPtr, ok := nestedStructType(field.Type())
		if !ok {
			bodyField.TypeExpr = types.TypeString(field.Type(), nil)
			bodyField.Opaque = true
			body.Fields = append(body.Fields, bodyField)
			continue
		}
		nested, ok := analyseBodyStruct(nestedType, "", bodyField.Required)
		if !ok {
			return nil, false
		}
		bodyField.Nested = nested
		bodyField.NestedType = types.TypeString(nestedType, nil)
		bodyField.NestedPtr = nestedPtr
		body.Fields = append(body.Fields, bodyField)
	}

	return body, true
}

func applyBodyFieldValidation(field *BodyFieldSpec, tag reflect.StructTag) bool {
	rules, err := validation.ParseFieldRules(tag)
	if err != nil {
		return false
	}
	for _, rule := range rules.Rules {
		if rule.Kind == validation.RulePattern {
			if _, err := regexp.Compile(rule.String); err != nil {
				return false
			}
		}
	}
	field.Rules = rules
	return true
}

func analyseOutputType(t types.Type) OutputWriteSpec {
	spec := OutputWriteSpec{Manual: true, StaticStatus: http.StatusOK}
	original := t
	named, ok := t.(*types.Named)
	if ok {
		t = named.Underlying()
	}
	strct, ok := t.(*types.Struct)
	if !ok {
		return spec
	}

	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if !field.Exported() {
			continue
		}
		tag := reflect.StructTag(strct.Tag(i))

		switch {
		case tag.Get("body") != "" || field.Name() == "Body":
			spec.BodyFieldName = field.Name()
			spec.BodyTypeExpr = types.TypeString(field.Type(), nil)
			bodySpec := analyseOutputBody(field.Type(), "out."+field.Name())
			if bodySpec != nil {
				bodySpec.HasSimpleStatus = spec.StaticStatus > 0 && spec.StatusField == ""
			}
			spec.Body = bodySpec
		case tag.Get("status") != "" || field.Name() == "Status":
			spec.StatusField = field.Name()
			spec.StaticStatus = 0
		case tag.Get("header") != "":
			typeExpr, kind, pointer, ok := supportedScalar(field.Type())
			if !ok {
				return OutputWriteSpec{}
			}
			spec.Headers = append(spec.Headers, HeaderFieldSpec{
				FieldName: field.Name(),
				Header:    tagName(tag.Get("header")),
				TypeExpr:  typeExpr,
				Kind:      kind,
				Pointer:   pointer,
				Required:  requiredForTag(tag, "header", field.Type()),
			})
		}
	}

	if spec.BodyFieldName == "" {
		bodySpec := analyseOutputBody(original, "out")
		if bodySpec != nil {
			bodySpec.HasSimpleStatus = spec.StaticStatus > 0 && spec.StatusField == ""
		}
		spec.Body = bodySpec
	}

	return spec
}

func analyseOutputBody(t types.Type, targetExpr string) *OutputBodySpec {
	underlying := t
	if named, ok := t.(*types.Named); ok {
		underlying = named.Underlying()
	}
	strct, ok := underlying.(*types.Struct)
	if !ok {
		return nil
	}

	fields := make([]OutputBodyFieldSpec, 0, strct.NumFields())
	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if !field.Exported() {
			continue
		}
		tag := reflect.StructTag(strct.Tag(i))
		if fieldSourceName, _ := fieldSource(tag); fieldSourceName != "" {
			return nil
		}
		jsonTag := tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			return nil
		}
		typeExpr, kind, pointer, ok := supportedScalar(field.Type())
		if !ok {
			return nil
		}
		jsonName := tagName(jsonTag)
		if jsonName == "" {
			jsonName = field.Name()
		}
		fields = append(fields, OutputBodyFieldSpec{
			FieldName: field.Name(),
			JSONName:  jsonName,
			TypeExpr:  typeExpr,
			Kind:      kind,
			Pointer:   pointer,
		})
	}
	if len(fields) == 0 {
		return nil
	}
	return &OutputBodySpec{TargetExpr: targetExpr, Fields: fields}
}

func fieldSource(tag reflect.StructTag) (string, string) {
	switch {
	case tag.Get("path") != "":
		return "path", tagName(tag.Get("path"))
	case tag.Get("query") != "":
		return "query", tagName(tag.Get("query"))
	case tag.Get("header") != "":
		return "header", tagName(tag.Get("header"))
	case tag.Get("cookie") != "":
		return "cookie", tagName(tag.Get("cookie"))
	default:
		return "", ""
	}
}

func supportedScalar(t types.Type) (typeExpr, kind string, pointer bool, ok bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		typeExpr, kind, _, ok := supportedScalar(ptr.Elem())
		return typeExpr, kind, true, ok
	}

	switch b := t.Underlying().(type) {
	case *types.Basic:
		switch b.Kind() {
		case types.String:
			return types.TypeString(t, nil), "string", false, true
		case types.Bool:
			return types.TypeString(t, nil), "bool", false, true
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64:
			return types.TypeString(t, nil), "int", false, true
		case types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
			return types.TypeString(t, nil), "uint", false, true
		case types.Float32, types.Float64:
			return types.TypeString(t, nil), "float", false, true
		}
	}
	return "", "", false, false
}

func requiredForTag(tag reflect.StructTag, source string, t types.Type) bool {
	if source == "path" {
		return true
	}
	if required := strings.ToLower(tag.Get("required")); required == "true" {
		return true
	} else if required == "false" {
		return false
	}
	if rules, err := validation.ParseFieldRules(tag); err == nil {
		if rules.Required {
			return true
		}
		if rules.OmitEmpty {
			return false
		}
	}

	raw := tag.Get(source)
	parts := strings.Split(raw, ",")
	if slices.Contains(parts[1:], "omitempty") {
		return false
	}
	_, isPtr := t.(*types.Pointer)
	return !isPtr
}

func fieldRequiredForJSONTag(tag reflect.StructTag, t types.Type) bool {
	if required := strings.ToLower(tag.Get("required")); required == "true" {
		return true
	} else if required == "false" {
		return false
	}
	if rules, err := validation.ParseFieldRules(tag); err == nil {
		if rules.Required {
			return true
		}
		if rules.OmitEmpty {
			return false
		}
	}
	raw := tag.Get("json")
	parts := strings.Split(raw, ",")
	if slices.Contains(parts[1:], "omitempty") {
		return false
	}
	_, isPtr := t.(*types.Pointer)
	return !isPtr
}

func mustParseRules(tag reflect.StructTag) validation.FieldRules {
	rules, err := validation.ParseFieldRules(tag)
	if err != nil {
		return validation.FieldRules{}
	}
	return rules
}

func hasRequiredJSONFieldsForTypesStruct(strct *types.Struct) bool {
	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if !field.Exported() {
			continue
		}
		tag := reflect.StructTag(strct.Tag(i))
		if fieldSourceName, _ := fieldSource(tag); fieldSourceName != "" {
			continue
		}
		if jsonTag := tag.Get("json"); jsonTag == "" || jsonTag == "-" {
			continue
		}
		if fieldRequiredForJSONTag(tag, field.Type()) {
			return true
		}
	}
	return false
}
