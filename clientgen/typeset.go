package clientgen

import (
	"fmt"
	"sort"
	"strings"
)

// typeSet assigns Go type declarations to the (inlined) schemas found while
// walking operations. Schemas are deduplicated by structural identity so a
// shape that appears in several operations — and as a named entry in
// components.schemas — collapses to a single Go type with a clean name.
type typeSet struct {
	doc            *Document
	byKey          map[string]string // structural key -> assigned Go type name
	structs        []*structType
	enums          []*enumType
	taken          map[string]bool
	componentNames map[string]string // structural key -> preferred name from components.schemas
	notices        []string
}

type structField struct {
	GoName   string
	JSONName string
	GoType   string
	Optional bool
	Doc      string
}

type structType struct {
	Name   string
	Fields []structField
	Doc    string
}

type enumType struct {
	Name   string
	Values []string
	Doc    string
}

func newTypeSet(doc *Document) *typeSet {
	ts := &typeSet{
		doc:            doc,
		byKey:          map[string]string{},
		taken:          map[string]bool{},
		componentNames: map[string]string{},
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

	// Composition is not modeled in V1; treat as opaque.
	if len(s.AllOf) > 0 || len(s.OneOf) > 0 || len(s.AnyOf) > 0 {
		ts.note("schema composition (allOf/oneOf/anyOf) at %s emitted as json.RawMessage", ctx)
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
	key := ts.canonical(s, nil)
	if name, ok := ts.byKey[key]; ok {
		return name
	}

	name := ts.componentNames[key]
	if name == "" {
		name = exportedName(ctx)
	}
	name = uniqueName(name, ts.taken)
	ts.byKey[key] = name // reserve before recursing (guards self-reference)

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
	key := ts.canonical(s, nil)
	if name, ok := ts.byKey[key]; ok {
		return name
	}
	name := ts.componentNames[key]
	if name == "" {
		name = exportedName(ctx)
	}
	name = uniqueName(name, ts.taken)
	ts.byKey[key] = name

	values := make([]string, 0, len(s.Enum))
	for _, v := range s.Enum {
		if str, ok := v.(string); ok {
			values = append(values, str)
		}
	}
	ts.enums = append(ts.enums, &enumType{Name: name, Values: values, Doc: strings.TrimSpace(s.Description)})
	return name
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
	case len(s.AllOf) > 0 || len(s.OneOf) > 0 || len(s.AnyOf) > 0:
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
	case s.Type == "string" && len(s.Enum) > 0:
		vals := make([]string, 0, len(s.Enum))
		for _, v := range s.Enum {
			vals = append(vals, fmt.Sprint(v))
		}
		sort.Strings(vals)
		return "enum:string:[" + strings.Join(vals, "|") + "]"
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
