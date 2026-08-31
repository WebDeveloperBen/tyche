package validation

import (
	"fmt"
	"reflect"
)

type FlattenedField struct {
	Field reflect.StructField
	Index []int
}

func FlattenFields(t reflect.Type) ([]FlattenedField, error) {
	t = IndirectType(t)
	if t == nil || t.Kind() != reflect.Struct {
		return nil, nil
	}

	fields := make([]FlattenedField, 0, t.NumField())
	seen := make(map[string]struct{})
	active := make(map[reflect.Type]bool)
	var visit func(reflect.Type, []int) error
	visit = func(current reflect.Type, prefix []int) error {
		current = IndirectType(current)
		if current == nil || current.Kind() != reflect.Struct {
			return nil
		}
		if active[current] {
			return nil
		}
		active[current] = true
		defer delete(active, current)

		for i := range current.NumField() {
			field := current.Field(i)
			if !field.IsExported() {
				continue
			}
			index := append(append([]int(nil), prefix...), i)
			if shouldFlattenField(field) {
				nested := IndirectType(field.Type)
				if nested != nil && nested.Kind() == reflect.Struct && !isScalarStruct(nested) && !active[nested] {
					if err := visit(nested, index); err != nil {
						return err
					}
					continue
				}
			}

			if source, name := fieldSource(field); source != "" {
				key := source + ":" + name
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate parameter %q in %s", name, source)
				}
				seen[key] = struct{}{}
			}
			fields = append(fields, FlattenedField{Field: field, Index: index})
		}
		return nil
	}
	if err := visit(t, nil); err != nil {
		return nil, err
	}
	return fields, nil
}

func shouldFlattenField(field reflect.StructField) bool {
	if !field.Anonymous || field.Name == "Body" {
		return false
	}
	return field.Tag.Get("path") == "" &&
		field.Tag.Get("query") == "" &&
		field.Tag.Get("header") == "" &&
		field.Tag.Get("cookie") == "" &&
		field.Tag.Get("form") == "" &&
		field.Tag.Get("file") == "" &&
		field.Tag.Get("files") == "" &&
		field.Tag.Get("body") == "" &&
		field.Tag.Get("json") == ""
}

func fieldSource(field reflect.StructField) (string, string) {
	switch {
	case field.Tag.Get("path") != "":
		return "path", TagName(field.Tag.Get("path"))
	case field.Tag.Get("query") != "":
		return "query", TagName(field.Tag.Get("query"))
	case field.Tag.Get("header") != "":
		return "header", TagName(field.Tag.Get("header"))
	case field.Tag.Get("cookie") != "":
		return "cookie", TagName(field.Tag.Get("cookie"))
	case field.Tag.Get("form") != "":
		return "form", TagName(field.Tag.Get("form"))
	case field.Tag.Get("file") != "":
		return "file", TagName(field.Tag.Get("file"))
	case field.Tag.Get("files") != "":
		return "files", TagName(field.Tag.Get("files"))
	default:
		return "", ""
	}
}
