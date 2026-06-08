package servergen

import (
	"bytes"
	"fmt"
	"go/types"
	"slices"
	"sort"
	"strconv"

	"github.com/webdeveloperben/tyche/server/validation"
)

func writeParseBody(buf *bytes.Buffer, route RouteSpec) {
	if !route.InputBind.Manual {
		buf.WriteString("\t\t\treturn serverpkg.ParseRequest[")
		buf.WriteString(route.InputType)
		buf.WriteString("](req)\n")
		return
	}

	buf.WriteString("\t\t\tvar in ")
	buf.WriteString(route.InputType)
	buf.WriteString("\n")
	buf.WriteString("\t\t\tvar validationErr serverpkg.ValidationError\n")
	for _, field := range route.InputBind.Fields {
		rawVar := "raw_" + field.FieldName
		pointer := validation.JSONPointer(field.Source, field.ParamName)
		switch field.Source {
		case "path":
			buf.WriteString("\t\t\t")
			buf.WriteString(rawVar)
			buf.WriteString(" := req.PathValue(")
			buf.WriteString(strconv.Quote(field.ParamName))
			buf.WriteString(")\n")
			if field.Required {
				buf.WriteString("\t\t\tif ")
				buf.WriteString(rawVar)
				buf.WriteString(" == \"\" {\n")
				buf.WriteString("\t\t\t\tvalidationErr.AddRequired(")
				buf.WriteString(strconv.Quote(pointer))
				buf.WriteString(")\n")
				buf.WriteString("\t\t\t}\n")
			}
			writeBindAssign(buf, "in."+field.FieldName, rawVar, field, pointer)
		case "query":
			buf.WriteString("\t\t\t")
			buf.WriteString(rawVar)
			buf.WriteString(" := req.URL.Query().Get(")
			buf.WriteString(strconv.Quote(field.ParamName))
			buf.WriteString(")\n")
			if field.Required {
				buf.WriteString("\t\t\tif ")
				buf.WriteString(rawVar)
				buf.WriteString(" == \"\" {\n")
				buf.WriteString("\t\t\t\tvalidationErr.AddRequired(")
				buf.WriteString(strconv.Quote(pointer))
				buf.WriteString(")\n")
				buf.WriteString("\t\t\t}\n")
			}
			writeBindAssign(buf, "in."+field.FieldName, rawVar, field, pointer)
		case "header":
			buf.WriteString("\t\t\t")
			buf.WriteString(rawVar)
			buf.WriteString(" := req.Header.Get(")
			buf.WriteString(strconv.Quote(field.ParamName))
			buf.WriteString(")\n")
			if field.Required {
				buf.WriteString("\t\t\tif ")
				buf.WriteString(rawVar)
				buf.WriteString(" == \"\" {\n")
				buf.WriteString("\t\t\t\tvalidationErr.AddRequired(")
				buf.WriteString(strconv.Quote(pointer))
				buf.WriteString(")\n")
				buf.WriteString("\t\t\t}\n")
			}
			writeBindAssign(buf, "in."+field.FieldName, rawVar, field, pointer)
		case "cookie":
			buf.WriteString("\t\t\tvar ")
			buf.WriteString(rawVar)
			buf.WriteString(" string\n")
			buf.WriteString("\t\t\tif cookie, err := req.Cookie(" + strconv.Quote(field.ParamName) + "); err == nil && cookie != nil {\n")
			buf.WriteString("\t\t\t\t" + rawVar + " = cookie.Value\n")
			buf.WriteString("\t\t\t} else if err != nil && !errors.Is(err, http.ErrNoCookie) {\n")
			buf.WriteString("\t\t\t\treturn nil, err\n")
			buf.WriteString("\t\t\t}\n")
			if field.Required {
				buf.WriteString("\t\t\tif " + rawVar + " == \"\" {\n")
				buf.WriteString("\t\t\t\tvalidationErr.AddRequired(" + strconv.Quote(pointer) + ")\n")
				buf.WriteString("\t\t\t}\n")
			}
			writeBindAssign(buf, "in."+field.FieldName, rawVar, field, pointer)
		}
	}
	if route.InputBind.Body != nil {
		writeGeneratedBodyParse(buf, route.InputBind.Body)
	}
	buf.WriteString("\t\t\tif !validationErr.Empty() { return nil, &validationErr }\n")
	buf.WriteString("\t\t\treturn &in, nil\n")
}

func writeGeneratedBodyParse(buf *bytes.Buffer, body *BodyBindSpec) {
	if body.Direct == nil && !bodyHasValidationRules(body) {
		writeStrictGeneratedBodyParse(buf, body)
		return
	}
	if canUseFastGeneratedBodyDecode(body) {
		buf.WriteString("\t\t\tbodyBytes, err := serverpkg.ReadRequestJSONBodyFast(req)\n")
		buf.WriteString("\t\t\tif err != nil { return nil, err }\n")
		buf.WriteString("\t\t\tif len(bodyBytes) == 0 {\n")
		if body.Required {
			buf.WriteString("\t\t\t\tvalidationErr.AddRequired(\"\")\n")
			buf.WriteString("\t\t\t\treturn nil, &validationErr\n")
		} else {
			buf.WriteString("\t\t\t\treturn &in, nil\n")
		}
		buf.WriteString("\t\t\t}\n")
		writeFastGeneratedBodyParse(buf, body)
		return
	}
	if canUseStrictWholeBodyDecode(body) {
		writeStrictWholeBodyParse(buf, body)
		return
	}
	buf.WriteString("\t\t\tbodyBytes, err := serverpkg.ReadRequestJSONBodyFast(req)\n")
	buf.WriteString("\t\t\tif err != nil { return nil, err }\n")
	buf.WriteString("\t\t\tif len(bodyBytes) == 0 {\n")
	if body.Required {
		buf.WriteString("\t\t\t\tvalidationErr.AddRequired(\"\")\n")
		buf.WriteString("\t\t\t\treturn nil, &validationErr\n")
	} else {
		buf.WriteString("\t\t\t\treturn &in, nil\n")
	}
	buf.WriteString("\t\t\t}\n")
	if body.Direct != nil {
		writeDirectBodyDecode(buf, *body.Direct, "bodyBytes", body.Target, "\t\t\t", "body_root", `""`)
		return
	}
	writeGeneratedBodyFields(buf, body.Fields, "raw_body", "bodyBytes", body.Target, "\t\t\t", "body", `""`)
}

func writeStrictGeneratedBodyParse(buf *bytes.Buffer, body *BodyBindSpec) {
	const indent = "\t\t\t"
	buf.WriteString(indent + "if err := serverpkg.ValidateJSONContentType(req.Header.Get(\"Content-Type\")); err != nil { return nil, err }\n")
	buf.WriteString(indent + "if req.Body == nil {\n")
	if body.Required {
		buf.WriteString(indent + "\tvalidationErr.AddRequired(\"\")\n")
		buf.WriteString(indent + "\treturn nil, &validationErr\n")
	} else {
		buf.WriteString(indent + "\treturn &in, nil\n")
	}
	buf.WriteString(indent + "}\n")
	buf.WriteString(indent + "var decoded ")
	writeDecodedBodyStructType(buf, body.Fields, "")
	buf.WriteString("\n")
	buf.WriteString(indent + "dec := json.NewDecoder(req.Body)\n")
	buf.WriteString(indent + "dec.DisallowUnknownFields()\n")
	buf.WriteString(indent + "if err := dec.Decode(&decoded); err != nil {\n")
	buf.WriteString(indent + "\tif errors.Is(err, io.EOF) {\n")
	if body.Required {
		buf.WriteString(indent + "\t\tvalidationErr.AddRequired(\"\")\n")
		buf.WriteString(indent + "\t\treturn nil, &validationErr\n")
	} else {
		buf.WriteString(indent + "\t\treturn &in, nil\n")
	}
	buf.WriteString(indent + "\t}\n")
	buf.WriteString(indent + "\treturn nil, fmt.Errorf(\"failed to decode body: %w\", err)\n")
	buf.WriteString(indent + "}\n")
	buf.WriteString(indent + "if err := serverpkg.EnsureSingleJSONValue(dec); err != nil { return nil, err }\n")
	writeDecodedBodyAssignments(buf, body.Fields, "decoded", body.Target, indent, strconv.Quote(""))
}

func writeDecodedBodyStructType(buf *bytes.Buffer, fields []BodyFieldSpec, indent string) {
	buf.WriteString("struct {\n")
	for _, field := range fields {
		buf.WriteString(indent + "\t" + field.FieldName + " ")
		writeDecodedBodyFieldType(buf, field, indent+"\t")
		buf.WriteString(" `json:" + strconv.Quote(field.JSONName) + "`\n")
	}
	buf.WriteString(indent + "}")
}

func writeDecodedBodyFieldType(buf *bytes.Buffer, field BodyFieldSpec, indent string) {
	switch {
	case field.Nested != nil:
		buf.WriteString("*")
		writeDecodedBodyStructType(buf, field.Nested.Fields, indent)
	case field.Slice && field.ElemNested != nil:
		buf.WriteString("*[]")
		writeDecodedBodyStructType(buf, field.ElemNested.Fields, indent)
	case field.Slice:
		buf.WriteString("*[]" + field.ElemType)
	default:
		buf.WriteString("*" + field.TypeExpr)
	}
}

func writeDecodedBodyAssignments(buf *bytes.Buffer, fields []BodyFieldSpec, sourceExpr, targetPrefix, indent, pointerBaseExpr string) {
	for _, field := range fields {
		fieldSource := sourceExpr + "." + field.FieldName
		fieldTarget := targetPrefix + field.FieldName
		fieldPointerExpr := "serverpkg.JoinValidationPointer(" + pointerBaseExpr + ", " + strconv.Quote(field.JSONName) + ")"
		buf.WriteString(indent + "if " + fieldSource + " == nil {\n")
		if field.Required {
			buf.WriteString(indent + "\tvalidationErr.AddRequired(" + fieldPointerExpr + ")\n")
		}
		buf.WriteString(indent + "} else {\n")
		switch {
		case field.Nested != nil:
			if field.Pointer {
				buf.WriteString(indent + "\tdecoded_" + field.FieldName + " := &" + field.NestedType + "{}\n")
				writeDecodedBodyAssignments(buf, field.Nested.Fields, "(*"+fieldSource+")", "decoded_"+field.FieldName+".", indent+"\t", fieldPointerExpr)
				buf.WriteString(indent + "\t" + fieldTarget + " = decoded_" + field.FieldName + "\n")
			} else {
				writeDecodedBodyAssignments(buf, field.Nested.Fields, "(*"+fieldSource+")", fieldTarget+".", indent+"\t", fieldPointerExpr)
			}
		case field.Slice && field.ElemNested != nil:
			buf.WriteString(indent + "\t" + fieldTarget + " = make([]" + field.ElemStruct + ", 0, len(*" + fieldSource + "))\n")
			buf.WriteString(indent + "\tfor i := range *" + fieldSource + " {\n")
			if field.ElemStructPtr {
				buf.WriteString(indent + "\t\tdecodedElem := &" + field.ElemStruct + "{}\n")
				writeDecodedBodyAssignments(buf, field.ElemNested.Fields, "(*"+fieldSource+")[i]", "decodedElem.", indent+"\t\t", "serverpkg.JoinValidationPointer("+fieldPointerExpr+", strconv.Itoa(i))")
				buf.WriteString(indent + "\t\t" + fieldTarget + " = append(" + fieldTarget + ", decodedElem)\n")
			} else {
				buf.WriteString(indent + "\t\tvar decodedElem " + field.ElemStruct + "\n")
				writeDecodedBodyAssignments(buf, field.ElemNested.Fields, "(*"+fieldSource+")[i]", "decodedElem.", indent+"\t\t", "serverpkg.JoinValidationPointer("+fieldPointerExpr+", strconv.Itoa(i))")
				buf.WriteString(indent + "\t\t" + fieldTarget + " = append(" + fieldTarget + ", decodedElem)\n")
			}
			buf.WriteString(indent + "\t}\n")
		case field.Slice:
			buf.WriteString(indent + "\t" + fieldTarget + " = *" + fieldSource + "\n")
		default:
			if field.Pointer {
				buf.WriteString(indent + "\t" + fieldTarget + " = " + fieldSource + "\n")
			} else {
				buf.WriteString(indent + "\t" + fieldTarget + " = *" + fieldSource + "\n")
			}
		}
		buf.WriteString(indent + "}\n")
	}
}

func writeStrictWholeBodyParse(buf *bytes.Buffer, body *BodyBindSpec) {
	required := RequiredJSONFieldsForBodySpec(body)
	buf.WriteString("\t\t\tif err := serverpkg.DecodeRequestJSONBodyStrictFast(req, &" + body.DecodeTarget + ", " + strconv.FormatBool(body.Required) + ", ")
	writeRequiredJSONFieldsLiteral(buf, required)
	buf.WriteString("); err != nil { return nil, err }\n")
}

func writeFastGeneratedBodyParse(buf *bytes.Buffer, body *BodyBindSpec) {
	buf.WriteString("\t\t\tvar decoded struct {\n")
	for _, field := range body.Fields {
		buf.WriteString("\t\t\t\t" + field.FieldName + " " + field.TypeExpr + " `json:" + strconv.Quote(field.JSONName) + "`\n")
	}
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\tif err := json.Unmarshal(bodyBytes, &decoded); err != nil { return nil, fmt.Errorf(\"failed to decode body: %w\", err) }\n")
	for _, field := range body.Fields {
		writeBodyValidation(buf, "decoded."+field.FieldName, strconv.Quote(validation.JSONPointer(field.JSONName)), field, "\t\t\t")
		buf.WriteString("\t\t\t" + body.Target + field.FieldName + " = decoded." + field.FieldName + "\n")
	}
}

func writeBodyValidation(buf *bytes.Buffer, valueExpr, pointerExpr string, field BodyFieldSpec, indent string) {
	writeRuleValidation(buf, valueExpr, pointerExpr, field.Kind, field.Rules, indent)
}

func canUseFastGeneratedBodyDecode(body *BodyBindSpec) bool {
	if body == nil || body.Direct != nil || len(body.Fields) == 0 {
		return false
	}
	for _, field := range body.Fields {
		if field.Slice || field.Opaque || field.Nested != nil || field.ElemNested != nil {
			return false
		}
		if field.TypeExpr == "" || field.Kind == "" {
			return false
		}
	}
	return true
}

func canUseStrictWholeBodyDecode(body *BodyBindSpec) bool {
	if body == nil || body.DecodeTarget == "" {
		return false
	}
	if body.Direct != nil {
		return false
	}
	return !bodyHasValidationRules(body)
}

func bodyHasValidationRules(body *BodyBindSpec) bool {
	if body == nil {
		return false
	}
	if body.Direct != nil {
		return len(body.Direct.Rules.Rules) > 0 || len(body.Direct.Rules.ItemRules) > 0
	}
	for _, field := range body.Fields {
		if len(field.Rules.Rules) > 0 || len(field.Rules.ItemRules) > 0 {
			return true
		}
		if bodyHasValidationRules(field.Nested) || bodyHasValidationRules(field.ElemNested) {
			return true
		}
	}
	return false
}

func RequiredJSONFieldsForBodySpec(body *BodyBindSpec) []BodyRequiredFieldSpec {
	if body == nil || body.Direct != nil {
		return nil
	}
	var required []BodyRequiredFieldSpec
	collectRequiredJSONFields(body.Fields, nil, nil, &required)
	return required
}

type BodyRequiredFieldSpec struct {
	Pointer []string
	Path    []string
}

func collectRequiredJSONFields(fields []BodyFieldSpec, pointerPrefix, pathPrefix []string, out *[]BodyRequiredFieldSpec) {
	for _, field := range fields {
		pointerPath := append(append([]string(nil), pointerPrefix...), field.JSONName)
		path := append(append([]string(nil), pathPrefix...), field.JSONName)
		if field.Required {
			*out = append(*out, BodyRequiredFieldSpec{Pointer: pointerPath, Path: path})
		}
		if !field.Required {
			continue
		}
		switch {
		case field.Nested != nil:
			collectRequiredJSONFields(field.Nested.Fields, pointerPath, path, out)
		case field.ElemNested != nil:
			collectRequiredJSONFields(field.ElemNested.Fields, pointerPath, append(path, "*"), out)
		}
	}
}

func writeRequiredJSONFieldsLiteral(buf *bytes.Buffer, required []BodyRequiredFieldSpec) {
	if len(required) == 0 {
		buf.WriteString("nil")
		return
	}
	buf.WriteString("[]serverpkg.RequiredJSONField{")
	for i, field := range required {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString("{Pointer: ")
		buf.WriteString(strconv.Quote(validation.JSONPointer(field.Pointer...)))
		buf.WriteString(", Path: []string{")
		for j, part := range field.Path {
			if j > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(strconv.Quote(part))
		}
		buf.WriteString("}}")
	}
	buf.WriteString("}")
}

func writeBindAssign(buf *bytes.Buffer, target, raw string, field BindFieldSpec, pointer string) {
	if !field.Required {
		buf.WriteString("\t\t\tif " + raw + " != \"\" {\n")
		writeBindAssignInner(buf, target, raw, field, "\t\t\t\t", strconv.Quote(pointer))
		buf.WriteString("\t\t\t}\n")
		return
	}
	writeBindAssignInner(buf, target, raw, field, "\t\t\t", strconv.Quote(pointer))
}

func writeBindAssignInner(buf *bytes.Buffer, target, raw string, field BindFieldSpec, indent string, pointerExpr string) {
	switch field.Kind {
	case "string":
		if field.Pointer {
			buf.WriteString(indent + "tmp := " + raw + "\n")
			buf.WriteString(indent + target + " = &tmp\n")
			writeRuleValidation(buf, "tmp", pointerExpr, field.Kind, field.Rules, indent)
		} else {
			buf.WriteString(indent + target + " = " + raw + "\n")
			writeRuleValidation(buf, raw, pointerExpr, field.Kind, field.Rules, indent)
		}
	case "bool":
		buf.WriteString(indent + "parsed, err := strconv.ParseBool(" + raw + ")\n")
		buf.WriteString(indent + "if err != nil { validationErr.AddInvalidType(" + pointerExpr + ") }\n")
		if field.Pointer {
			buf.WriteString(indent + "tmp := " + field.TypeExpr + "(parsed)\n")
			buf.WriteString(indent + target + " = &tmp\n")
			writeRuleValidation(buf, "tmp", pointerExpr, field.Kind, field.Rules, indent)
		} else {
			buf.WriteString(indent + target + " = " + field.TypeExpr + "(parsed)\n")
			writeRuleValidation(buf, target, pointerExpr, field.Kind, field.Rules, indent)
		}
	case "int":
		buf.WriteString(indent + "parsed, err := strconv.ParseInt(" + raw + ", 10, 64)\n")
		buf.WriteString(indent + "if err != nil { validationErr.AddInvalidType(" + pointerExpr + ") }\n")
		if field.Pointer {
			buf.WriteString(indent + "tmp := " + field.TypeExpr + "(parsed)\n")
			buf.WriteString(indent + target + " = &tmp\n")
			writeRuleValidation(buf, "tmp", pointerExpr, field.Kind, field.Rules, indent)
		} else {
			buf.WriteString(indent + target + " = " + field.TypeExpr + "(parsed)\n")
			writeRuleValidation(buf, target, pointerExpr, field.Kind, field.Rules, indent)
		}
	case "uint":
		buf.WriteString(indent + "parsed, err := strconv.ParseUint(" + raw + ", 10, 64)\n")
		buf.WriteString(indent + "if err != nil { validationErr.AddInvalidType(" + pointerExpr + ") }\n")
		if field.Pointer {
			buf.WriteString(indent + "tmp := " + field.TypeExpr + "(parsed)\n")
			buf.WriteString(indent + target + " = &tmp\n")
			writeRuleValidation(buf, "tmp", pointerExpr, field.Kind, field.Rules, indent)
		} else {
			buf.WriteString(indent + target + " = " + field.TypeExpr + "(parsed)\n")
			writeRuleValidation(buf, target, pointerExpr, field.Kind, field.Rules, indent)
		}
	case "float":
		buf.WriteString(indent + "parsed, err := strconv.ParseFloat(" + raw + ", 64)\n")
		buf.WriteString(indent + "if err != nil { validationErr.AddInvalidType(" + pointerExpr + ") }\n")
		if field.Pointer {
			buf.WriteString(indent + "tmp := " + field.TypeExpr + "(parsed)\n")
			buf.WriteString(indent + target + " = &tmp\n")
			writeRuleValidation(buf, "tmp", pointerExpr, field.Kind, field.Rules, indent)
		} else {
			buf.WriteString(indent + target + " = " + field.TypeExpr + "(parsed)\n")
			writeRuleValidation(buf, target, pointerExpr, field.Kind, field.Rules, indent)
		}
	}
}

func writeRuleValidation(buf *bytes.Buffer, valueExpr, pointerExpr, kind string, rules validation.FieldRules, indent string) {
	for _, rule := range rules.Rules {
		switch rule.Kind {
		case validation.RuleMin:
			switch kind {
			case "string":
				buf.WriteString(indent + "if serverpkg.ValidationStringLength(" + valueExpr + ") < " + strconv.Itoa(rule.Int) + " { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleMin, serverpkg.ValidationSubjectString, " + strconv.Itoa(rule.Int) + ") }\n")
			case "int", "uint", "float":
				buf.WriteString(indent + "if " + valueExpr + " < " + strconv.Itoa(rule.Int) + " { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleMin, serverpkg.ValidationSubjectNumber, " + strconv.Itoa(rule.Int) + ") }\n")
			}
		case validation.RuleMax:
			switch kind {
			case "string":
				buf.WriteString(indent + "if serverpkg.ValidationStringLength(" + valueExpr + ") > " + strconv.Itoa(rule.Int) + " { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleMax, serverpkg.ValidationSubjectString, " + strconv.Itoa(rule.Int) + ") }\n")
			case "int", "uint", "float":
				buf.WriteString(indent + "if " + valueExpr + " > " + strconv.Itoa(rule.Int) + " { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleMax, serverpkg.ValidationSubjectNumber, " + strconv.Itoa(rule.Int) + ") }\n")
			}
		case validation.RuleLen:
			switch kind {
			case "string":
				buf.WriteString(indent + "if serverpkg.ValidationStringLength(" + valueExpr + ") != " + strconv.Itoa(rule.Int) + " { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleLen, serverpkg.ValidationSubjectString, " + strconv.Itoa(rule.Int) + ") }\n")
			case "int", "uint", "float":
				buf.WriteString(indent + "if " + valueExpr + " != " + strconv.Itoa(rule.Int) + " { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleLen, serverpkg.ValidationSubjectNumber, " + strconv.Itoa(rule.Int) + ") }\n")
			}
		case validation.RuleMinItems:
			buf.WriteString(indent + "if len(" + valueExpr + ") < " + strconv.Itoa(rule.Int) + " { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleMinItems, serverpkg.ValidationSubjectCollection, " + strconv.Itoa(rule.Int) + ") }\n")
		case validation.RuleMaxItems:
			buf.WriteString(indent + "if len(" + valueExpr + ") > " + strconv.Itoa(rule.Int) + " { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleMaxItems, serverpkg.ValidationSubjectCollection, " + strconv.Itoa(rule.Int) + ") }\n")
		case validation.RuleOneOf:
			if kind != "string" {
				continue
			}
			buf.WriteString(indent + "switch " + valueExpr + " {\n")
			for _, val := range rule.List {
				buf.WriteString(indent + "case " + strconv.Quote(val) + ":\n")
			}
			buf.WriteString(indent + "default:\n")
			buf.WriteString(indent + "\tvalidationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleOneOf, serverpkg.ValidationSubjectString, 0)\n")
			buf.WriteString(indent + "}\n")
		case validation.RulePattern:
			if kind != "string" {
				continue
			}
			buf.WriteString(indent + "if !" + patternVarName(rule.String) + ".MatchString(" + valueExpr + ") { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRulePattern, serverpkg.ValidationSubjectString, 0) }\n")
		case validation.RuleEmail:
			if kind != "string" {
				continue
			}
			buf.WriteString(indent + "if _, err := mail.ParseAddress(" + valueExpr + "); err != nil { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleEmail, serverpkg.ValidationSubjectString, 0) }\n")
		case validation.RuleURL:
			if kind != "string" {
				continue
			}
			buf.WriteString(indent + "if parsed, err := url.ParseRequestURI(" + valueExpr + "); err != nil || parsed == nil || parsed.Scheme == \"\" || parsed.Host == \"\" { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleURL, serverpkg.ValidationSubjectString, 0) }\n")
		case validation.RuleUUID:
			if kind != "string" {
				continue
			}
			buf.WriteString(indent + "if !serverpkg.ValidateUUID(" + valueExpr + ") { validationErr.AddRule(" + pointerExpr + ", serverpkg.ValidationRuleUUID, serverpkg.ValidationSubjectString, 0) }\n")
		}
	}
}

func writeWriteBody(buf *bytes.Buffer, route RouteSpec) {
	if !route.OutputWrite.Manual {
		writeFallbackWrite(buf, route)
		return
	}

	buf.WriteString("\t\t\tif value == nil {\n")
	buf.WriteString("\t\t\t\tvar out *" + route.OutputType + "\n")
	buf.WriteString("\t\t\t\treturn serverpkg.WriteTypedResponse[" + route.OutputType + "](w, out)\n")
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\tout, ok := value.(*" + route.OutputType + ")\n")
	buf.WriteString("\t\t\tif !ok {\n")
	buf.WriteString("\t\t\t\treturn fmt.Errorf(\"generated codec " + route.OperationID + " expected *" + route.OutputType + "\")\n")
	buf.WriteString("\t\t\t}\n")

	if route.OutputWrite.StatusField != "" {
		buf.WriteString("\t\t\tstatus := out." + route.OutputWrite.StatusField + "\n")
	} else if route.OutputWrite.StaticStatus > 0 {
		buf.WriteString("\t\t\tconst status = " + strconv.Itoa(route.OutputWrite.StaticStatus) + "\n")
	} else {
		buf.WriteString("\t\t\tstatus := " + strconv.Itoa(route.OutputWrite.StaticStatus) + "\n")
	}

	for _, header := range route.OutputWrite.Headers {
		if header.Pointer {
			buf.WriteString("\t\t\tif out." + header.FieldName + " != nil {\n")
			buf.WriteString("\t\t\t\tw.Header().Set(" + strconv.Quote(header.Header) + ", fmt.Sprint(*out." + header.FieldName + "))\n")
			buf.WriteString("\t\t\t}\n")
		} else {
			buf.WriteString("\t\t\tw.Header().Set(" + strconv.Quote(header.Header) + ", fmt.Sprint(out." + header.FieldName + "))\n")
		}
	}
	if route.OutputWrite.StatusField != "" {
		buf.WriteString("\t\t\tif req != nil && req.Method == http.MethodHead { w.WriteHeader(status); return nil }\n")
		buf.WriteString("\t\t\tif (status >= 100 && status < 200) || status == http.StatusNoContent || status == http.StatusNotModified { w.WriteHeader(status); return nil }\n")
	} else if route.OutputWrite.StaticStatus > 0 && route.OutputWrite.Body == nil {
		buf.WriteString("\t\t\tw.WriteHeader(status)\n")
	} else if route.OutputWrite.StaticStatus > 0 {
		// No separate WriteHeader needed - body writes its own
	} else {
		buf.WriteString("\t\t\tif req != nil && req.Method == http.MethodHead { w.WriteHeader(status); return nil }\n")
		buf.WriteString("\t\t\tif (status >= 100 && status < 200) || status == http.StatusNoContent || status == http.StatusNotModified { w.WriteHeader(status); return nil }\n")
	}
	if route.OutputWrite.Body != nil {
		writeGeneratedOutputBody(buf, route.OutputWrite.Body, route.OutputWrite.StaticStatus)
		return
	}
	if route.OutputWrite.BodyFieldName != "" {
		buf.WriteString("\t\t\treturn serverpkg.WriteJSON(w, status, out." + route.OutputWrite.BodyFieldName + ")\n")
		return
	}
	buf.WriteString("\t\t\treturn serverpkg.WriteJSON(w, status, out)\n")
}

func writeGeneratedOutputBody(buf *bytes.Buffer, body *OutputBodySpec, staticStatus int) {
	buf.WriteString("\t\t\tbufPtr := serverpkg.AcquireGeneratedJSONBuffer()\n")
	buf.WriteString("\t\t\tdefer serverpkg.ReleaseGeneratedJSONBuffer(bufPtr)\n")
	buf.WriteString("\t\t\tb := (*bufPtr)[:0]\n")
	for i, field := range body.Fields {
		prefix := `{"` + field.JSONName + `":`
		if i > 0 {
			prefix = `,"` + field.JSONName + `":`
		}
		buf.WriteString("\t\t\tb = append(b, " + strconv.Quote(prefix) + "...)\n")
		valueExpr := body.TargetExpr + "." + field.FieldName
		if field.Pointer {
			buf.WriteString("\t\t\tif " + valueExpr + " == nil {\n")
			buf.WriteString("\t\t\t\tb = append(b, \"null\"...)\n")
			buf.WriteString("\t\t\t} else {\n")
			writeAppendJSONValue(buf, "*"+valueExpr, field.Kind, "\t\t\t\t")
			buf.WriteString("\t\t\t}\n")
			continue
		}
		writeAppendJSONValue(buf, valueExpr, field.Kind, "\t\t\t")
	}
	buf.WriteString("\t\t\tb = append(b, '}', '\\n')\n")
	buf.WriteString("\t\t\tw.Header().Set(\"Content-Type\", \"application/json\")\n")
	if staticStatus > 0 && body.HasSimpleStatus {
		buf.WriteString("\t\t\tw.WriteHeader(" + strconv.Itoa(staticStatus) + ")\n")
	} else {
		buf.WriteString("\t\t\tw.WriteHeader(status)\n")
	}
	buf.WriteString("\t\t\t_, writeErr := w.Write(b)\n")
	buf.WriteString("\t\t\treturn writeErr\n")
}

func writeAppendJSONValue(buf *bytes.Buffer, valueExpr, kind, indent string) {
	switch kind {
	case "string":
		buf.WriteString(indent + "b = strconv.AppendQuote(b, " + valueExpr + ")\n")
	case "bool":
		buf.WriteString(indent + "b = strconv.AppendBool(b, " + valueExpr + ")\n")
	case "int":
		buf.WriteString(indent + "b = strconv.AppendInt(b, int64(" + valueExpr + "), 10)\n")
	case "uint":
		buf.WriteString(indent + "b = strconv.AppendUint(b, uint64(" + valueExpr + "), 10)\n")
	case "float":
		buf.WriteString(indent + "b = strconv.AppendFloat(b, float64(" + valueExpr + "), 'f', -1, 64)\n")
	}
}

func writeFallbackWrite(buf *bytes.Buffer, route RouteSpec) {
	buf.WriteString("\t\t\tif value == nil {\n")
	buf.WriteString("\t\t\t\tw.WriteHeader(http.StatusNoContent)\n")
	buf.WriteString("\t\t\t\treturn nil\n")
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\tout, ok := value.(*" + route.OutputType + ")\n")
	buf.WriteString("\t\t\tif !ok {\n")
	buf.WriteString("\t\t\t\treturn fmt.Errorf(\"generated codec " + route.OperationID + " expected *" + route.OutputType + "\")\n")
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\tw.Header().Set(\"Content-Type\", \"application/json\")\n")
	buf.WriteString("\t\t\tw.WriteHeader(http.StatusOK)\n")
	buf.WriteString("\t\t\treturn json.NewEncoder(w).Encode(out)\n")
}

func nestedStructType(t types.Type) (types.Type, bool, bool) {
	if ptr, ok := t.(*types.Pointer); ok {
		nested, _, ok := nestedStructType(ptr.Elem())
		return nested, true, ok
	}
	underlying := t
	if named, ok := t.(*types.Named); ok {
		underlying = named.Underlying()
	}
	_, ok := underlying.(*types.Struct)
	return t, false, ok
}

func sliceElementType(t types.Type) (types.Type, bool) {
	switch v := t.(type) {
	case *types.Slice:
		return v.Elem(), true
	case *types.Named:
		return sliceElementType(v.Underlying())
	default:
		return nil, false
	}
}

func markGeneratedBodyImports(body *BodyBindSpec, useFmt, useRegexp, useNetMail, useNetURL *bool) {
	if body.Direct != nil {
		markValidationImports(body.Direct.Rules, useFmt, useRegexp, useNetMail, useNetURL)
		if body.Direct.ElemNested != nil {
			markGeneratedBodyImports(body.Direct.ElemNested, useFmt, useRegexp, useNetMail, useNetURL)
		}
	}
	for _, field := range body.Fields {
		markValidationImports(field.Rules, useFmt, useRegexp, useNetMail, useNetURL)
		if field.Nested != nil {
			markGeneratedBodyImports(field.Nested, useFmt, useRegexp, useNetMail, useNetURL)
		}
		if field.ElemNested != nil {
			markGeneratedBodyImports(field.ElemNested, useFmt, useRegexp, useNetMail, useNetURL)
		}
	}
}

func generatedBodyNeedsStrconv(body *BodyBindSpec) bool {
	if body == nil {
		return false
	}
	if body.Direct != nil {
		return bodyFieldNeedsStrconv(*body.Direct)
	}
	return slices.ContainsFunc(body.Fields, bodyFieldNeedsStrconv)
}

func bodyFieldNeedsStrconv(field BodyFieldSpec) bool {
	if field.Slice && (len(field.Rules.ItemRules) > 0 || field.ElemNested != nil) {
		return true
	}
	if field.Nested != nil && generatedBodyNeedsStrconv(field.Nested) {
		return true
	}
	if field.ElemNested != nil && generatedBodyNeedsStrconv(field.ElemNested) {
		return true
	}
	return false
}

func writePatternVars(buf *bytes.Buffer, routes []RouteSpec) {
	patterns := make(map[string]struct{})
	for _, route := range routes {
		for _, field := range route.InputBind.Fields {
			collectValidationPatterns(patterns, field.Rules)
		}
		if route.InputBind.Body != nil {
			collectBodyPatterns(patterns, route.InputBind.Body)
		}
	}
	if len(patterns) == 0 {
		return
	}

	keys := make([]string, 0, len(patterns))
	for pattern := range patterns {
		keys = append(keys, pattern)
	}
	sort.Strings(keys)

	buf.WriteString("var (\n")
	for _, pattern := range keys {
		buf.WriteString("\t" + patternVarName(pattern) + " = regexp.MustCompile(" + strconv.Quote(pattern) + ")\n")
	}
	buf.WriteString(")\n\n")
}

func collectBodyPatterns(patterns map[string]struct{}, body *BodyBindSpec) {
	if body.Direct != nil {
		collectValidationPatterns(patterns, body.Direct.Rules)
		if body.Direct.ElemNested != nil {
			collectBodyPatterns(patterns, body.Direct.ElemNested)
		}
	}
	for _, field := range body.Fields {
		collectValidationPatterns(patterns, field.Rules)
		if field.Nested != nil {
			collectBodyPatterns(patterns, field.Nested)
		}
		if field.ElemNested != nil {
			collectBodyPatterns(patterns, field.ElemNested)
		}
	}
}

func markValidationImports(rules validation.FieldRules, useFmt, useRegexp, useNetMail, useNetURL *bool) {
	if (len(rules.Rules) > 0 || len(rules.ItemRules) > 0) && useFmt != nil {
		*useFmt = true
	}
	allRules := append(append([]validation.Rule(nil), rules.Rules...), rules.ItemRules...)
	for _, rule := range allRules {
		switch rule.Kind {
		case validation.RulePattern:
			if useRegexp != nil {
				*useRegexp = true
			}
		case validation.RuleEmail:
			if useNetMail != nil {
				*useNetMail = true
			}
		case validation.RuleURL:
			if useNetURL != nil {
				*useNetURL = true
			}
		}
	}
}

func collectValidationPatterns(patterns map[string]struct{}, rules validation.FieldRules) {
	for _, rule := range rules.Rules {
		if rule.Kind == validation.RulePattern {
			patterns[rule.String] = struct{}{}
		}
	}
}

func patternVarName(pattern string) string {
	var hash uint64 = 1469598103934665603
	for _, r := range pattern {
		hash ^= uint64(r)
		hash *= 1099511628211
	}
	return fmt.Sprintf("generatedPattern_%x", hash)
}

func writeAllowedFieldSet(buf *bytes.Buffer, rawVar string, fields []BodyFieldSpec, indent string) {
	allowedVar := "allowed_" + rawVar
	buf.WriteString(indent + allowedVar + " := map[string]struct{}{\n")
	for _, field := range fields {
		buf.WriteString(indent + "\t" + strconv.Quote(field.JSONName) + ": {},\n")
	}
	buf.WriteString(indent + "}\n")
}

func writeGeneratedBodyFields(buf *bytes.Buffer, fields []BodyFieldSpec, rawVar, rawValueExpr, targetPrefix, indent, pathPrefix, pointerBaseExpr string) {
	writeAllowedFieldSet(buf, rawVar, fields, indent)
	objectVar := "obj_" + rawVar
	buf.WriteString(indent + "var " + objectVar + " map[string]json.RawMessage\n")
	buf.WriteString(indent + "if err := json.Unmarshal(" + rawValueExpr + ", &" + objectVar + "); err != nil { return nil, fmt.Errorf(\"failed to decode body: %w\", err) }\n")
	buf.WriteString(indent + "for key := range " + objectVar + " {\n")
	buf.WriteString(indent + "\tif _, ok := " + objectVar + "[key]; !ok { continue }\n")
	buf.WriteString(indent + "\tif _, ok := " + "allowed_" + rawVar + "[key]; !ok { return nil, fmt.Errorf(\"failed to decode body: json: unknown field %q\", key) }\n")
	buf.WriteString(indent + "}\n")
	for _, field := range fields {
		fieldRawVar := "raw_" + pathPrefix + "_" + field.FieldName
		fieldPointerExpr := "serverpkg.JoinValidationPointer(" + pointerBaseExpr + ", " + strconv.Quote(field.JSONName) + ")"
		buf.WriteString(indent + fieldRawVar + ", ok := " + objectVar + "[" + strconv.Quote(field.JSONName) + "]\n")
		if field.Required {
			buf.WriteString(indent + "if !ok { validationErr.AddRequired(" + fieldPointerExpr + "); return nil, &validationErr }\n")
		} else {
			buf.WriteString(indent + "if !ok {\n")
			buf.WriteString(indent + "\t// Optional field omitted.\n")
			buf.WriteString(indent + "} else {\n")
		}
		if field.Nested != nil {
			writeNestedBodyField(buf, field, fieldRawVar, targetPrefix+field.FieldName, indent+"\t", pathPrefix+"_"+field.FieldName, fieldPointerExpr)
		} else if field.Slice {
			writeSliceBodyField(buf, field, fieldRawVar, targetPrefix+field.FieldName, indent+"\t", pathPrefix+"_"+field.FieldName, fieldPointerExpr)
		} else {
			writeDirectBodyDecode(buf, field, fieldRawVar, targetPrefix+field.FieldName, indent+"\t", pathPrefix+"_"+field.FieldName, fieldPointerExpr)
		}
		if !field.Required {
			buf.WriteString(indent + "}\n")
		}
	}
}

func writeDirectBodyDecode(buf *bytes.Buffer, field BodyFieldSpec, rawValueExpr, target, indent, pathKey, pointerExpr string) {
	if field.Pointer {
		buf.WriteString(indent + "var tmp_" + pathKey + " " + field.TypeExpr + "\n")
		buf.WriteString(indent + "if err := json.Unmarshal(" + rawValueExpr + ", &tmp_" + pathKey + "); err != nil { return nil, fmt.Errorf(\"failed to decode body: %w\", err) }\n")
		writeBodyValidation(buf, "tmp_"+pathKey, pointerExpr, field, indent)
		buf.WriteString(indent + target + " = &tmp_" + pathKey + "\n")
		return
	}
	buf.WriteString(indent + "if err := json.Unmarshal(" + rawValueExpr + ", &" + target + "); err != nil { return nil, fmt.Errorf(\"failed to decode body: %w\", err) }\n")
	writeBodyValidation(buf, target, pointerExpr, field, indent)
}

func writeNestedBodyField(buf *bytes.Buffer, field BodyFieldSpec, rawFieldVar, target, indent, pathKey, pointerExpr string) {
	if field.Pointer {
		buf.WriteString(indent + "tmp_" + pathKey + " := &" + field.NestedType + "{}\n")
		writeGeneratedBodyFields(buf, field.Nested.Fields, "raw_"+pathKey, rawFieldVar, "tmp_"+pathKey+".", indent, pathKey, pointerExpr)
		buf.WriteString(indent + target + " = tmp_" + pathKey + "\n")
		return
	}
	writeGeneratedBodyFields(buf, field.Nested.Fields, "raw_"+pathKey, rawFieldVar, target+".", indent, pathKey, pointerExpr)
}

func writeSliceBodyField(buf *bytes.Buffer, field BodyFieldSpec, rawFieldVar, target, indent, pathKey, pointerExpr string) {
	if field.ElemNested != nil {
		writeNestedSliceBodyField(buf, field, rawFieldVar, target, indent, pathKey, pointerExpr)
		return
	}

	buf.WriteString(indent + "if err := json.Unmarshal(" + rawFieldVar + ", &" + target + "); err != nil { return nil, fmt.Errorf(\"failed to decode body: %w\", err) }\n")
	loopVar := "i_" + pathKey
	elemExpr := target + "[" + loopVar + "]"
	buf.WriteString(indent)
	buf.WriteString("for ")
	buf.WriteString(loopVar)
	buf.WriteString(" := range ")
	buf.WriteString(target)
	buf.WriteString(" {\n")
	if field.ElemPtr {
		buf.WriteString(indent)
		buf.WriteString("\tif ")
		buf.WriteString(elemExpr)
		buf.WriteString(" != nil {\n")
		elemField := BodyFieldSpec{
			JSONName: field.JSONName,
			Kind:     field.ElemKind,
			Rules:    validation.FieldRules{Rules: field.Rules.ItemRules},
		}
		writeBodyValidation(buf, "*"+elemExpr, "serverpkg.JoinValidationPointer("+pointerExpr+", strconv.Itoa("+loopVar+"))", elemField, indent+"\t\t")
		buf.WriteString(indent)
		buf.WriteString("\t}\n")
	} else {
		elemField := BodyFieldSpec{
			JSONName: field.JSONName,
			Kind:     field.ElemKind,
			Rules:    validation.FieldRules{Rules: field.Rules.ItemRules},
		}
		writeBodyValidation(buf, elemExpr, "serverpkg.JoinValidationPointer("+pointerExpr+", strconv.Itoa("+loopVar+"))", elemField, indent+"\t")
	}
	buf.WriteString(indent)
	buf.WriteString("}\n")
}

func writeNestedSliceBodyField(buf *bytes.Buffer, field BodyFieldSpec, rawFieldVar, target, indent, pathKey, pointerExpr string) {
	buf.WriteString(indent)
	buf.WriteString("var rawlist_")
	buf.WriteString(pathKey)
	buf.WriteString(" []json.RawMessage\n")
	buf.WriteString(indent)
	buf.WriteString("if err := json.Unmarshal(")
	buf.WriteString(rawFieldVar)
	buf.WriteString(", &rawlist_")
	buf.WriteString(pathKey)
	buf.WriteString("); err != nil { return nil, fmt.Errorf(\"failed to decode body: %w\", err) }\n")
	buf.WriteString(indent)
	buf.WriteString(target)
	buf.WriteString(" = make([]")
	buf.WriteString(field.ElemStruct)
	buf.WriteString(", 0, len(rawlist_")
	buf.WriteString(pathKey)
	buf.WriteString("))\n")
	buf.WriteString(indent)
	buf.WriteString("for i_")
	buf.WriteString(pathKey)
	buf.WriteString(" := range rawlist_")
	buf.WriteString(pathKey)
	buf.WriteString(" {\n")
	buf.WriteString(indent)
	buf.WriteString("\tvar elemtmp_")
	buf.WriteString(pathKey)
	buf.WriteString(" ")
	buf.WriteString(field.ElemStruct)
	buf.WriteString("\n")
	writeGeneratedBodyFields(buf, field.ElemNested.Fields, "raw_"+pathKey+"_elem", "rawlist_"+pathKey+"[i_"+pathKey+"]", "elemtmp_"+pathKey+".", indent+"\t", pathKey+"_elem", "serverpkg.JoinValidationPointer("+pointerExpr+", strconv.Itoa(i_"+pathKey+"))")
	buf.WriteString(indent)
	buf.WriteString("\t")
	buf.WriteString(target)
	buf.WriteString(" = append(")
	buf.WriteString(target)
	buf.WriteString(", elemtmp_")
	buf.WriteString(pathKey)
	buf.WriteString(")\n")
	buf.WriteString(indent)
	buf.WriteString("}\n")
}
