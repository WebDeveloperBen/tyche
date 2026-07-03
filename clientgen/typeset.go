package clientgen

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// typeSet assigns Go type declarations to the (inlined) schemas found while
// walking operations. By default, schemas are deduplicated by structural
// identity so a shape that appears in several operations — and as a named entry
// in components.schemas — collapses to a single Go type with a clean name.
type typeSet struct {
	doc            *Document
	byKey          map[string]string
	taken          map[string]bool
	componentNames map[string]string
	building       map[*Schema]string
	structs        []*structType
	enums          []*enumType
	notices        []string
	naming         TypeNamingStrategy
}

type structField struct {
	GoName   string
	JSONName string
	GoType   string
	Doc      string
	Optional bool
}

type structType struct {
	Name   string
	Doc    string
	Fields []structField
}

type enumType struct {
	Name   string
	Base   string
	Doc    string
	Values []string
}

func newTypeSet(doc *Document, naming TypeNamingStrategy) *typeSet {
	ts := &typeSet{
		doc:            doc,
		naming:         naming,
		byKey:          map[string]string{},
		taken:          map[string]bool{},
		componentNames: map[string]string{},
		building:       map[*Schema]string{},
	}
	// Recover clean names for the structures that appear as named components.
	if doc.Components != nil {
		keys := make([]string, 0, len(doc.Components.Schemas))
		for k := range doc.Components.Schemas {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			s := doc.Components.Schemas[k]
			sk := ts.canonical(s, nil)
			if sk == "" {
				continue
			}
			if _, ok := ts.componentNames[sk]; !ok {
				ts.componentNames[sk] = componentBaseName(k)
			}
		}
	}
	return ts
}

// goType returns the Go type expression for schema s, registering named struct
// and enum declarations as a side effect. ctx is a fallback name (e.g.
// "CreateUserInputBody") used when the structure has no component name.
func (ts *typeSet) goType(s *Schema, ctx string) string {
	s = ts.doc.resolve(s)
	if s == nil {
		return "json.RawMessage"
	}

	// oneOf/anyOf are unions, which Go doesn't model naturally — keep opaque.
	if len(s.OneOf) > 0 || len(s.AnyOf) > 0 {
		ts.note("oneOf/anyOf composition at %s emitted as json.RawMessage", ctx)
		return "json.RawMessage"
	}
	// allOf composing objects is merged into a single struct; a lone member
	// (e.g. allOf:[{$ref}] used to attach a description) is unwrapped.
	if len(s.AllOf) > 0 {
		if merged, ok := ts.flattenAllOf(s, nil); ok {
			return ts.namedStruct(merged, ctx)
		}
		if len(s.AllOf) == 1 && len(s.Properties) == 0 {
			return ts.goType(s.AllOf[0], ctx)
		}
		ts.note("allOf composition at %s is not a mergeable object; emitted as json.RawMessage", ctx)
		return "json.RawMessage"
	}

	switch {
	case len(s.Properties) > 0:
		return ts.namedStruct(s, ctx)
	case s.Type == "array":
		return "[]" + ts.goType(s.Items, ctx+"Item")
	case s.Type == "object" || s.AdditionalProperties != nil:
		return ts.mapType(s, ctx)
	case s.Type == "string" && len(s.Enum) > 0:
		return ts.namedEnum(s, ctx)
	case s.Type == "integer" && len(s.Enum) > 0:
		return ts.namedEnum(s, ctx)
	case s.Type == "string":
		return stringType(s.Format)
	case s.Type == "integer":
		return integerType(s.Format)
	case s.Type == "number":
		return numberType(s.Format)
	case s.Type == "boolean":
		return "bool"
	default:
		return "json.RawMessage"
	}
}

func (ts *typeSet) mapType(s *Schema, ctx string) string {
	if s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil {
		return "map[string]" + ts.goType(s.AdditionalProperties.Schema, ctx+"Value")
	}
	// Free-form object (additionalProperties: true / absent).
	return "json.RawMessage"
}

func (ts *typeSet) namedStruct(s *Schema, ctx string) string {
	s = ts.doc.resolve(s)
	if s == nil {
		return "json.RawMessage"
	}
	if name, ok := ts.building[s]; ok {
		return name
	}

	structuralKey := ts.canonical(s, nil)
	key := ts.typeKey(structuralKey, ctx)
	if name, ok := ts.byKey[key]; ok {
		return name
	}

	name := ts.typeName(structuralKey, ctx)
	if name == "" {
		name = exportedName(ctx)
	}
	name = uniqueName(name, ts.taken)
	ts.byKey[key] = name // reserve before recursing (guards self-reference)
	ts.building[s] = name
	defer delete(ts.building, s)

	st := &structType{Name: name, Doc: strings.TrimSpace(s.Description)}
	required := map[string]bool{}
	for _, r := range s.Required {
		required[r] = true
	}

	propNames := make([]string, 0, len(s.Properties))
	for p := range s.Properties {
		propNames = append(propNames, p)
	}
	sort.Strings(propNames)

	fieldTaken := map[string]bool{}
	for _, jsonName := range propNames {
		ps := s.Properties[jsonName]
		goName := uniqueName(exportedName(jsonName), fieldTaken)
		optional := !required[jsonName]
		st.Fields = append(st.Fields, structField{
			GoName:   goName,
			JSONName: jsonName,
			GoType:   ts.goType(ps, name+goName),
			Optional: optional,
			Doc:      strings.TrimSpace(ts.doc.resolve(ps).Description),
		})
	}

	ts.structs = append(ts.structs, st)
	return name
}

func (ts *typeSet) namedEnum(s *Schema, ctx string) string {
	s = ts.doc.resolve(s)
	if s == nil {
		return "json.RawMessage"
	}
	structuralKey := ts.canonical(s, nil)
	key := ts.typeKey(structuralKey, ctx)
	if name, ok := ts.byKey[key]; ok {
		return name
	}
	name := ts.typeName(structuralKey, ctx)
	if name == "" {
		name = exportedName(ctx)
	}
	name = uniqueName(name, ts.taken)
	ts.byKey[key] = name

	base := "string"
	if s.Type == "integer" {
		base = integerType(s.Format)
	}
	values := make([]string, 0, len(s.Enum))
	for _, v := range s.Enum {
		if lit, ok := enumLiteral(v, base); ok {
			values = append(values, lit)
		}
	}
	ts.enums = append(ts.enums, &enumType{Name: name, Base: base, Values: values, Doc: strings.TrimSpace(s.Description)})
	return name
}

func (ts *typeSet) typeKey(structuralKey, ctx string) string {
	if ts.naming == TypeNamingOperationScoped {
		return exportedName(ctx) + "\x00" + structuralKey
	}
	return structuralKey
}

func (ts *typeSet) typeName(structuralKey, ctx string) string {
	if ts.naming == TypeNamingOperationScoped {
		return exportedName(ctx)
	}
	return ts.componentNames[structuralKey]
}

// enumLiteral renders an enum value for a Go constant of the given base type.
// For string enums it returns the raw string (quoted at emit time); for integer
// enums it returns the integer literal. The bool is false when the value can't
// be represented (e.g. a non-integer in an integer enum), so it's skipped.
func enumLiteral(v any, base string) (string, bool) {
	if base == "string" {
		s, ok := v.(string)
		return s, ok
	}
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case int:
		return strconv.Itoa(n), true
	case string:
		if _, err := strconv.ParseInt(n, 10, 64); err == nil {
			return n, true
		}
	}
	return "", false
}

// flattenAllOf merges an allOf composition into a single synthetic object
// schema: the union of every member's properties and required lists, resolving
// $ref members and recursing into nested allOf. It returns (merged, true) only
// when the composition is cleanly an object — a member that is a scalar, array,
// map, enum, or oneOf/anyOf makes it non-mergeable and yields (nil, false), so
// the caller falls back to json.RawMessage. On a property-name collision the
// first member wins.
func (ts *typeSet) flattenAllOf(s *Schema, seen map[*Schema]bool) (*Schema, bool) {
	if seen == nil {
		seen = map[*Schema]bool{}
	}
	merged := &Schema{Type: "object", Properties: map[string]*Schema{}, Description: strings.TrimSpace(s.Description)}
	reqSeen := map[string]bool{}
	addReq := func(names []string) {
		for _, r := range names {
			if !reqSeen[r] {
				reqSeen[r] = true
				merged.Required = append(merged.Required, r)
			}
		}
	}
	addProps := func(props map[string]*Schema) {
		for name, ps := range props {
			if _, exists := merged.Properties[name]; !exists {
				merged.Properties[name] = ps
			}
		}
	}

	// allOf may coexist with the schema's own inline properties.
	addProps(s.Properties)
	addReq(s.Required)

	for _, sub := range s.AllOf {
		sub = ts.doc.resolve(sub)
		if sub == nil {
			continue
		}
		if seen[sub] {
			return nil, false // ref cycle
		}
		seen[sub] = true
		ok := func() bool {
			switch {
			case len(sub.OneOf) > 0 || len(sub.AnyOf) > 0 || len(sub.Enum) > 0:
				return false
			case len(sub.AllOf) > 0:
				nested, ok := ts.flattenAllOf(sub, seen)
				if !ok {
					return false
				}
				addProps(nested.Properties)
				addReq(nested.Required)
			case len(sub.Properties) > 0:
				addProps(sub.Properties)
				addReq(sub.Required)
			case sub.Type == "object" && sub.AdditionalProperties == nil:
				addReq(sub.Required) // empty object contributes nothing but required
			default:
				return false // scalar, array, or free-form map
			}
			return true
		}()
		delete(seen, sub)
		if !ok {
			return nil, false
		}
	}

	if len(merged.Properties) == 0 {
		return nil, false
	}
	sort.Strings(merged.Required)
	return merged, true
}

func (ts *typeSet) note(format string, args ...any) {
	ts.notices = append(ts.notices, fmt.Sprintf(format, args...))
}

// canonical produces a deterministic structural key for a schema, ignoring
// descriptions/names, so structurally identical shapes share a key.
func (ts *typeSet) canonical(s *Schema, seen map[*Schema]bool) string {
	s = ts.doc.resolve(s)
	if s == nil {
		return "any"
	}
	if seen == nil {
		seen = map[*Schema]bool{}
	}
	if seen[s] {
		return "cycle"
	}
	seen[s] = true
	defer delete(seen, s)

	switch {
	case len(s.OneOf) > 0 || len(s.AnyOf) > 0:
		return "composite"
	case len(s.AllOf) > 0:
		if merged, ok := ts.flattenAllOf(s, nil); ok {
			return ts.canonical(merged, seen)
		}
		if len(s.AllOf) == 1 && len(s.Properties) == 0 {
			return ts.canonical(s.AllOf[0], seen)
		}
		return "composite"
	case len(s.Properties) > 0:
		names := make([]string, 0, len(s.Properties))
		for p := range s.Properties {
			names = append(names, p)
		}
		sort.Strings(names)
		req := map[string]bool{}
		for _, r := range s.Required {
			req[r] = true
		}
		var b strings.Builder
		b.WriteString("obj{")
		for _, n := range names {
			b.WriteString(n)
			if req[n] {
				b.WriteString("!")
			}
			b.WriteString(":")
			b.WriteString(ts.canonical(s.Properties[n], seen))
			b.WriteString(",")
		}
		b.WriteString("}")
		return b.String()
	case s.Type == "array":
		return "arr<" + ts.canonical(s.Items, seen) + ">"
	case s.Type == "object" || s.AdditionalProperties != nil:
		if s.AdditionalProperties != nil && s.AdditionalProperties.Schema != nil {
			return "map<" + ts.canonical(s.AdditionalProperties.Schema, seen) + ">"
		}
		return "raw"
	case (s.Type == "string" || s.Type == "integer") && len(s.Enum) > 0:
		vals := make([]string, 0, len(s.Enum))
		for _, v := range s.Enum {
			vals = append(vals, fmt.Sprint(v))
		}
		sort.Strings(vals)
		return "enum:" + s.Type + ":[" + strings.Join(vals, "|") + "]"
	default:
		return "scalar:" + s.Type + "/" + s.Format
	}
}

func stringType(format string) string {
	switch format {
	case "date-time", "date":
		return "time.Time"
	case "binary", "byte":
		return "[]byte"
	default:
		return "string"
	}
}

func integerType(format string) string {
	switch format {
	case "int64":
		return "int64"
	case "int32":
		return "int32"
	default:
		return "int"
	}
}

func numberType(format string) string {
	if format == "float" {
		return "float32"
	}
	return "float64"
}

// scalarParamType maps a parameter schema (path/query/header) to a Go type and
// reports whether it is a string-like value. Parameters are scalars or string
// slices in tyche.
func (ts *typeSet) scalarParamType(s *Schema) string {
	s = ts.doc.resolve(s)
	if s == nil {
		return "string"
	}
	if s.Type == "array" {
		return "[]" + ts.scalarParamType(s.Items)
	}
	switch s.Type {
	case "integer":
		return integerType(s.Format)
	case "number":
		return numberType(s.Format)
	case "boolean":
		return "bool"
	case "string":
		return stringType(s.Format)
	default:
		return "string"
	}
}
