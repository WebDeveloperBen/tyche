package clientgen

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// inputField describes one field of a generated <Op>Input struct.
type inputField struct {
	GoName   string
	WireName string // path/query/header parameter name
	In       string // "path" | "query" | "header" | "body" | "form" | "file" | "files"
	GoType   string
	Optional bool
}

func (f inputField) isSlice() bool { return strings.HasPrefix(f.GoType, "[]") }

// operation is a fully resolved operation ready for emission.
type operation struct {
	Method     string
	HTTPPath   string
	GoName     string
	InputName  string
	OutputType string
	EventType  string
	Summary    string
	Fields     []inputField
	Stream     bool
	Bytes      bool
	Deprecated bool
}

// buildOperations walks the document and resolves every operation, registering
// body/output types in the typeSet and returning the operations in a stable
// order.
func buildOperations(ts *typeSet, doc *Document) []*operation {
	paths := make([]string, 0, len(doc.Paths))
	for p := range doc.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	methodNames := map[string]bool{}
	var ops []*operation
	for _, p := range paths {
		item := doc.Paths[p]
		if item == nil {
			continue
		}
		for _, mo := range item.methods() {
			ops = append(ops, resolveOperation(ts, doc, p, mo.method, mo.op, methodNames))
		}
	}
	return ops
}

func resolveOperation(ts *typeSet, doc *Document, path, method string, op *Operation, methodNames map[string]bool) *operation {
	name := op.OperationID
	if name == "" {
		name = strings.ToLower(method) + "-" + path
	}
	goName := uniqueName(exportedName(name), methodNames)

	o := &operation{
		Method:     method,
		HTTPPath:   path,
		GoName:     goName,
		InputName:  goName + "Input",
		Summary:    strings.TrimSpace(op.Summary),
		Deprecated: op.Deprecated,
	}

	fieldTaken := map[string]bool{}
	for _, param := range op.Parameters {
		if param == nil || (param.In != "path" && param.In != "query" && param.In != "header") {
			continue
		}
		o.Fields = append(o.Fields, inputField{
			GoName:   uniqueName(exportedName(param.Name), fieldTaken),
			WireName: param.Name,
			In:       param.In,
			GoType:   ts.scalarParamType(param.Schema),
			Optional: param.In != "path" && !param.Required,
		})
	}

	if op.RequestBody != nil {
		if mt := op.RequestBody.Content["application/json"]; mt != nil && mt.Schema != nil {
			bodyType := ts.goType(mt.Schema, goName+"Body")
			o.Fields = append(o.Fields, inputField{
				GoName:   uniqueName("Body", fieldTaken),
				In:       "body",
				GoType:   pointerize(bodyType),
				Optional: !op.RequestBody.Required,
			})
		} else if mt := op.RequestBody.Content["multipart/form-data"]; mt != nil && mt.Schema != nil {
			o.Fields = append(o.Fields, multipartFields(ts, doc, mt.Schema, fieldTaken)...)
		}
	}

	if ev, ok := streamEvent(op); ok {
		o.Stream = true
		o.EventType = ts.goType(ev, goName+"Event")
	} else if data, ok := successData(doc, op); ok {
		o.OutputType = ts.goType(data, goName+"Output")
	} else if successBytes(op) {
		o.Bytes = true
	}
	return o
}

func multipartFields(ts *typeSet, doc *Document, schema *Schema, fieldTaken map[string]bool) []inputField {
	s := doc.resolve(schema)
	if s == nil || len(s.Properties) == 0 {
		return nil
	}
	required := map[string]bool{}
	for _, name := range s.Required {
		required[name] = true
	}
	names := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]inputField, 0, len(names))
	for _, name := range names {
		prop := doc.resolve(s.Properties[name])
		if prop == nil {
			continue
		}
		f := inputField{
			GoName:   uniqueName(exportedName(name), fieldTaken),
			WireName: name,
			In:       "form",
			GoType:   ts.scalarParamType(prop),
			Optional: !required[name],
		}
		if isBinarySchema(prop) {
			f.In = "file"
			f.GoType = "*File"
		} else if prop.Type == "array" && isBinarySchema(doc.resolve(prop.Items)) {
			f.In = "files"
			f.GoType = "[]File"
		}
		fields = append(fields, f)
	}
	return fields
}

func isBinarySchema(s *Schema) bool {
	return s != nil && s.Type == "string" && s.Format == "binary"
}

// successBytes reports whether the lowest 2xx response carries a body in a
// single non-JSON, non-SSE media type (e.g. application/octet-stream, text/*).
// Such a response is returned to the caller as raw bytes rather than being
// JSON-decoded — or, as before this, silently discarded.
func successBytes(op *Operation) bool {
	resp := lowest2xx(op)
	if resp == nil || len(resp.Content) == 0 {
		return false
	}
	if resp.Content["application/json"] != nil || resp.Content["text/event-stream"] != nil {
		return false
	}
	return true
}

// lowest2xx returns the response for the lowest 2xx status code, or nil.
func lowest2xx(op *Operation) *Response {
	codes := make([]string, 0, len(op.Responses))
	for code := range op.Responses {
		if len(code) > 0 && code[0] == '2' {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return nil
	}
	sort.Strings(codes)
	return op.Responses[codes[0]]
}

// streamEvent returns the event-data schema for a text/event-stream success
// response, and whether the operation is a streaming one.
func streamEvent(op *Operation) (*Schema, bool) {
	resp := lowest2xx(op)
	if resp == nil {
		return nil, false
	}
	if mt := resp.Content["text/event-stream"]; mt != nil && mt.Schema != nil {
		return mt.Schema, true
	}
	return nil, false
}

// successData returns the response body schema (unwrapped from the tyche
// {"data": …} envelope) for the lowest 2xx response, and whether one exists.
func successData(doc *Document, op *Operation) (*Schema, bool) {
	resp := lowest2xx(op)
	if resp == nil {
		return nil, false
	}
	mt := resp.Content["application/json"]
	if mt == nil || mt.Schema == nil {
		return nil, false
	}
	s := doc.resolve(mt.Schema)
	if s != nil && s.Properties != nil {
		if data, ok := s.Properties["data"]; ok {
			return data, true
		}
	}
	return mt.Schema, true
}

// pointerize makes a type nilable so optional/unset bodies can be distinguished.
func pointerize(goType string) string {
	if strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[") || strings.HasPrefix(goType, "*") {
		return goType
	}
	return "*" + goType
}

// emitInputStruct writes the <Op>Input type declaration.
func emitInputStruct(b *strings.Builder, o *operation) {
	fmt.Fprintf(b, "// %s is the input for [Client.%s].\n", o.InputName, o.GoName)
	fmt.Fprintf(b, "type %s struct {\n", o.InputName)
	for _, f := range o.Fields {
		goType := f.GoType
		if f.Optional && !f.isSlice() && !strings.HasPrefix(goType, "*") && f.In != "body" {
			goType = "*" + goType
		}
		switch f.In {
		case "body":
			fmt.Fprintf(b, "\t%s %s `json:\"-\"`\n", f.GoName, goType)
		case "form", "file", "files":
			fmt.Fprintf(b, "\t%s %s `%s:%q`\n", f.GoName, goType, f.In, f.WireName)
		default:
			fmt.Fprintf(b, "\t%s %s `%s:%q`\n", f.GoName, goType, f.In, f.WireName)
		}
	}
	b.WriteString("}\n\n")
}

// emitMethod writes the client method for an operation.
func emitMethod(b *strings.Builder, clientName string, o *operation) {
	if o.Summary != "" {
		fmt.Fprintf(b, "// %s: %s\n", o.GoName, o.Summary)
	} else {
		fmt.Fprintf(b, "// %s calls %s %s.\n", o.GoName, o.Method, o.HTTPPath)
	}
	if o.Deprecated {
		b.WriteString("//\n// Deprecated: this operation is marked deprecated in the API spec.\n")
	}

	ret := "error"
	switch {
	case o.Stream:
		ret = "(*Stream[" + o.EventType + "], error)"
	case o.OutputType != "":
		ret = "(*" + o.OutputType + ", error)"
	case o.Bytes:
		ret = "([]byte, error)"
	}
	fmt.Fprintf(b, "func (c *%s) %s(ctx context.Context, in *%s, opts ...CallOption) %s {\n", clientName, o.GoName, o.InputName, ret)
	b.WriteString("\tif in == nil {\n\t\tin = &" + o.InputName + "{}\n\t}\n")

	queryArg, headerArg, bodyArg := emitRequestBuild(b, o)

	httpMethod := "http.Method" + methodConst(o.Method)
	switch {
	case o.Stream:
		fmt.Fprintf(b, "\treturn doStream[%s](ctx, c, %s, path, %s, %s, %s, opts)\n", o.EventType, httpMethod, queryArg, headerArg, bodyArg)
	case o.OutputType != "":
		fmt.Fprintf(b, "\treturn doJSON[%s](ctx, c, %s, path, %s, %s, %s, opts)\n", o.OutputType, httpMethod, queryArg, headerArg, bodyArg)
	case o.Bytes:
		fmt.Fprintf(b, "\treturn doBytes(ctx, c, %s, path, %s, %s, %s, opts)\n", httpMethod, queryArg, headerArg, bodyArg)
	default:
		fmt.Fprintf(b, "\treturn doDiscard(ctx, c, %s, path, %s, %s, %s, opts)\n", httpMethod, queryArg, headerArg, bodyArg)
	}
	b.WriteString("}\n\n")
}

// emitRequestBuild writes the path/query/header/body construction shared by all
// method kinds and returns the call arguments for query, header, and body.
func emitRequestBuild(b *strings.Builder, o *operation) (queryArg, headerArg, bodyArg string) {
	fmt.Fprintf(b, "\tpath := %s\n", pathExpression(o.HTTPPath, o.Fields))

	queryArg, headerArg, bodyArg = "nil", "nil", "nil"

	if anyIn(o.Fields, "query") {
		b.WriteString("\tquery := url.Values{}\n")
		for _, f := range o.Fields {
			if f.In == "query" {
				emitParamSet(b, "query", f)
			}
		}
		queryArg = "query"
	}

	if anyIn(o.Fields, "header") {
		b.WriteString("\theader := http.Header{}\n")
		for _, f := range o.Fields {
			if f.In == "header" {
				emitParamSet(b, "header", f)
			}
		}
		headerArg = "header"
	}

	if f, ok := bodyField(o.Fields); ok {
		b.WriteString("\tvar body any\n")
		fmt.Fprintf(b, "\tif in.%s != nil {\n\t\tbody = in.%s\n\t}\n", f.GoName, f.GoName)
		bodyArg = "body"
	} else if hasMultipartFields(o.Fields) {
		b.WriteString("\tbody := &multipartBody{}\n")
		for _, f := range o.Fields {
			switch f.In {
			case "form":
				emitMultipartFieldSet(b, f)
			case "file":
				fmt.Fprintf(b, "\tif in.%s != nil {\n\t\tbody.addFile(%q, *in.%s)\n\t}\n", f.GoName, f.WireName, f.GoName)
			case "files":
				fmt.Fprintf(b, "\tfor _, file := range in.%s {\n\t\tbody.addFile(%q, file)\n\t}\n", f.GoName, f.WireName)
			}
		}
		bodyArg = "body"
	}
	return queryArg, headerArg, bodyArg
}

func emitMultipartFieldSet(b *strings.Builder, f inputField) {
	switch {
	case f.isSlice():
		fmt.Fprintf(b, "\tfor _, v := range in.%s {\n\t\tbody.addField(%q, fmtParam(v))\n\t}\n", f.GoName, f.WireName)
	case f.Optional:
		fmt.Fprintf(b, "\tif in.%s != nil {\n\t\tbody.addField(%q, fmtParam(*in.%s))\n\t}\n", f.GoName, f.WireName, f.GoName)
	default:
		fmt.Fprintf(b, "\tbody.addField(%q, fmtParam(in.%s))\n", f.WireName, f.GoName)
	}
}

func emitParamSet(b *strings.Builder, target string, f inputField) {
	switch {
	case f.isSlice():
		fmt.Fprintf(b, "\tfor _, v := range in.%s {\n\t\t%s.Add(%q, fmtParam(v))\n\t}\n", f.GoName, target, f.WireName)
	case f.Optional:
		fmt.Fprintf(b, "\tif in.%s != nil {\n\t\t%s.Set(%q, fmtParam(*in.%s))\n\t}\n", f.GoName, target, f.WireName, f.GoName)
	default:
		fmt.Fprintf(b, "\t%s.Set(%q, fmtParam(in.%s))\n", target, f.WireName, f.GoName)
	}
}

func pathExpression(path string, fields []inputField) string {
	fieldOf := map[string]string{}
	for _, f := range fields {
		if f.In == "path" {
			fieldOf[f.WireName] = f.GoName
		}
	}
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return `"/"`
	}
	parts := strings.Split(trimmed, "/")

	// Build minimal pieces, coalescing consecutive literal segments into a
	// single quoted string so "/users/{id}/posts" becomes
	// `"/users/" + url.PathEscape(...) + "/posts"`.
	var pieces []string
	lit := "/"
	for i, p := range parts {
		last := i == len(parts)-1
		if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
			pieces = append(pieces, strconv.Quote(lit))
			pieces = append(pieces, "url.PathEscape(fmtParam(in."+fieldOf[p[1:len(p)-1]]+"))")
			lit = ""
			if !last {
				lit = "/"
			}
			continue
		}
		lit += p
		if !last {
			lit += "/"
		}
	}
	if lit != "" {
		pieces = append(pieces, strconv.Quote(lit))
	}
	return strings.Join(pieces, " + ")
}

func anyIn(fields []inputField, in string) bool {
	for _, f := range fields {
		if f.In == in {
			return true
		}
	}
	return false
}

func hasStreaming(ops []*operation) bool {
	for _, o := range ops {
		if o.Stream {
			return true
		}
	}
	return false
}

func hasMultipartFields(fields []inputField) bool {
	for _, f := range fields {
		if f.In == "form" || f.In == "file" || f.In == "files" {
			return true
		}
	}
	return false
}

func bodyField(fields []inputField) (inputField, bool) {
	for _, f := range fields {
		if f.In == "body" {
			return f, true
		}
	}
	return inputField{}, false
}

func methodConst(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "Get"
	case "POST":
		return "Post"
	case "PUT":
		return "Put"
	case "PATCH":
		return "Patch"
	case "DELETE":
		return "Delete"
	case "HEAD":
		return "Head"
	case "OPTIONS":
		return "Options"
	case "TRACE":
		return "Trace"
	default:
		return "Get"
	}
}
