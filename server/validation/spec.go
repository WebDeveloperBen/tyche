package validation

import (
	"container/list"
	"fmt"
	"mime/multipart"
	"reflect"
	"sync"
)

type StructSpec struct {
	Type   reflect.Type
	Fields []FieldSpec
}

type FieldSpec struct {
	Type        reflect.Type
	ElementType reflect.Type
	Nested      *StructSpec
	ElemNested  *StructSpec
	Name        string
	Pointer     string
	FullPointer string
	Rules       FieldRules
	Index       int
	HasParam    bool
	Required    bool
}

type cachedSpec struct {
	spec *StructSpec
	err  error
}

const maxSpecCacheSize = 1024

type lruCache struct {
	cache map[reflect.Type]*list.Element
	order *list.List
	mu    sync.Mutex
}

func newLRUCache() *lruCache {
	return &lruCache{
		cache: make(map[reflect.Type]*list.Element),
		order: list.New(),
	}
}

func (c *lruCache) Get(t reflect.Type) (*cachedSpec, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.cache[t]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*cachedSpec), true
}

func (c *lruCache) Put(t reflect.Type, entry *cachedSpec) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[t]; ok {
		c.order.MoveToFront(elem)
		elem.Value = entry
		return
	}

	if c.order.Len() >= maxSpecCacheSize {
		elem := c.order.Back()
		if elem != nil {
			delete(c.cache, elem.Value.(*cachedSpec).spec.Type)
			c.order.Remove(elem)
		}
	}

	elem := c.order.PushFront(entry)
	c.cache[t] = elem
}

var structSpecCache = newLRUCache()

func Struct(t reflect.Type) (*StructSpec, error) {
	t = IndirectType(t)
	if cached, ok := structSpecCache.Get(t); ok {
		return cached.spec, cached.err
	}
	spec, err := buildStructSpec(t)
	entry := &cachedSpec{spec: spec, err: err}
	structSpecCache.Put(t, entry)
	return entry.spec, entry.err
}

func buildStructSpec(t reflect.Type) (*StructSpec, error) {
	t = IndirectType(t)
	spec := &StructSpec{Type: t}
	if t.Kind() != reflect.Struct || isScalarStruct(t) {
		return spec, nil
	}
	if err := validateBodyModeCompatibility(t); err != nil {
		return nil, err
	}

	spec.Fields = make([]FieldSpec, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		rules, err := ParseFieldRules(f.Tag)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		if err := validateRuleCompatibility(f, rules); err != nil {
			return nil, err
		}
		if err := validateMultipartFieldCompatibility(f); err != nil {
			return nil, err
		}

		fieldSpec := FieldSpec{
			Index:       i,
			Name:        fieldNameForValidation(f),
			Type:        f.Type,
			HasParam:    HasParamTag(f),
			Required:    fieldRequiredByValue(f),
			Rules:       rules,
			Pointer:     fieldPointer(f),
			ElementType: collectionElementType(f.Type),
		}

		nestedType := IndirectType(f.Type)
		if nestedType.Kind() == reflect.Struct && !isScalarStruct(nestedType) {
			nested, err := Struct(nestedType)
			if err != nil {
				return nil, err
			}
			fieldSpec.Nested = nested
		}
		elemType := IndirectType(fieldSpec.ElementType)
		if elemType != nil && elemType.Kind() == reflect.Struct && !isScalarStruct(elemType) {
			nested, err := Struct(elemType)
			if err != nil {
				return nil, err
			}
			fieldSpec.ElemNested = nested
		}
		spec.Fields = append(spec.Fields, fieldSpec)
	}

	for i := range spec.Fields {
		if i == 0 {
			spec.Fields[i].FullPointer = spec.Fields[i].Pointer
		} else {
			spec.Fields[i].FullPointer = JoinPointer(spec.Fields[0].Pointer, spec.Fields[i].Pointer)
		}
	}

	return spec, nil
}

func validateBodyModeCompatibility(t reflect.Type) error {
	var hasMultipart bool
	var hasJSONBody bool
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		if HasMultipartTag(f) {
			hasMultipart = true
		}
		if f.Tag.Get("body") != "" || f.Name == "Body" || IsJSONBodyField(f) {
			hasJSONBody = true
		}
	}
	if hasMultipart && hasJSONBody {
		return fmt.Errorf("%s: multipart form/file fields cannot be combined with JSON body fields", t.Name())
	}
	return nil
}

func IndirectType(t reflect.Type) reflect.Type {
	if t == nil {
		return nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func HasParamTag(f reflect.StructField) bool {
	return f.Tag.Get("path") != "" || f.Tag.Get("query") != "" || f.Tag.Get("header") != "" || f.Tag.Get("cookie") != ""
}

func IsJSONBodyField(f reflect.StructField) bool {
	_, ok := JSONFieldName(f)
	return ok && !HasParamTag(f) && !HasMultipartTag(f)
}

func HasMultipartTag(f reflect.StructField) bool {
	return f.Tag.Get("form") != "" || f.Tag.Get("file") != "" || f.Tag.Get("files") != ""
}

func fieldNameForValidation(f reflect.StructField) string {
	switch {
	case f.Tag.Get("path") != "":
		return TagName(f.Tag.Get("path"))
	case f.Tag.Get("query") != "":
		return TagName(f.Tag.Get("query"))
	case f.Tag.Get("header") != "":
		return TagName(f.Tag.Get("header"))
	case f.Tag.Get("cookie") != "":
		return TagName(f.Tag.Get("cookie"))
	case f.Tag.Get("form") != "":
		return TagName(f.Tag.Get("form"))
	case f.Tag.Get("file") != "":
		return TagName(f.Tag.Get("file"))
	case f.Tag.Get("files") != "":
		return TagName(f.Tag.Get("files"))
	default:
		if name, ok := JSONFieldName(f); ok {
			return name
		}
	}
	return f.Name
}

func fieldRequiredByValue(f reflect.StructField) bool {
	switch {
	case f.Tag.Get("path") != "":
		return true
	case f.Tag.Get("query") != "":
		return FieldRequired(f, "query")
	case f.Tag.Get("header") != "":
		return FieldRequired(f, "header")
	case f.Tag.Get("cookie") != "":
		return FieldRequired(f, "cookie")
	case f.Tag.Get("form") != "":
		return FieldRequired(f, "form")
	case f.Tag.Get("file") != "":
		return FieldRequired(f, "file")
	case f.Tag.Get("files") != "":
		return FieldRequired(f, "files")
	case f.Tag.Get("body") != "" || f.Name == "Body":
		return FieldRequired(f, "json")
	default:
		_, ok := JSONFieldName(f)
		return ok && FieldRequired(f, "json")
	}
}

func validateRuleCompatibility(f reflect.StructField, rules FieldRules) error {
	t := IndirectType(f.Type)
	for _, rule := range rules.Rules {
		switch rule.Kind {
		case RuleMin, RuleMax, RuleLen:
			switch t.Kind() {
			case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Float32, reflect.Float64:
			default:
				return fmt.Errorf("%s: validation rule %q is not supported for %s", f.Name, rule.Kind, t)
			}
		case RuleMinItems, RuleMaxItems:
			switch t.Kind() {
			case reflect.Slice, reflect.Array, reflect.Map:
			default:
				return fmt.Errorf("%s: validation rule %q is only supported for collections", f.Name, rule.Kind)
			}
		case RuleOneOf, RulePattern, RuleEmail, RuleURL, RuleUUID:
			if t.Kind() != reflect.String {
				return fmt.Errorf("%s: validation rule %q is only supported for strings", f.Name, rule.Kind)
			}
		}
	}
	elemType := IndirectType(collectionElementType(f.Type))
	for _, rule := range rules.ItemRules {
		switch rule.Kind {
		case RuleMin, RuleMax, RuleLen:
			switch elemType.Kind() {
			case reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
				reflect.Float32, reflect.Float64:
			default:
				return fmt.Errorf("%s: item validation rule %q is not supported for %s", f.Name, rule.Kind, elemType)
			}
		case RuleMinItems, RuleMaxItems:
			switch elemType.Kind() {
			case reflect.Slice, reflect.Array, reflect.Map:
			default:
				return fmt.Errorf("%s: item validation rule %q is only supported for collections", f.Name, rule.Kind)
			}
		case RuleOneOf, RulePattern, RuleEmail, RuleURL, RuleUUID:
			if elemType.Kind() != reflect.String {
				return fmt.Errorf("%s: item validation rule %q is only supported for strings", f.Name, rule.Kind)
			}
		}
	}
	return nil
}

func validateMultipartFieldCompatibility(f reflect.StructField) error {
	switch {
	case f.Tag.Get("file") != "":
		if f.Type != reflect.TypeFor[*multipart.FileHeader]() {
			return fmt.Errorf("%s: file fields must use *multipart.FileHeader", f.Name)
		}
	case f.Tag.Get("files") != "":
		if f.Type != reflect.TypeFor[[]*multipart.FileHeader]() {
			return fmt.Errorf("%s: files fields must use []*multipart.FileHeader", f.Name)
		}
	case f.Tag.Get("form") != "":
		t := IndirectType(f.Type)
		if t.Kind() == reflect.Slice {
			t = IndirectType(t.Elem())
		}
		switch t.Kind() {
		case reflect.String, reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
		default:
			return fmt.Errorf("%s: form fields must use scalar values or slices of scalar values", f.Name)
		}
	}
	return nil
}

func isScalarStruct(t reflect.Type) bool {
	return (t.PkgPath() == "time" && t.Name() == "Time") || t == reflect.TypeFor[multipart.FileHeader]()
}

func collectionElementType(t reflect.Type) reflect.Type {
	t = IndirectType(t)
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return t.Elem()
	default:
		return nil
	}
}

func fieldPointer(f reflect.StructField) string {
	switch {
	case f.Tag.Get("path") != "":
		return JSONPointer("path", TagName(f.Tag.Get("path")))
	case f.Tag.Get("query") != "":
		return JSONPointer("query", TagName(f.Tag.Get("query")))
	case f.Tag.Get("header") != "":
		return JSONPointer("header", TagName(f.Tag.Get("header")))
	case f.Tag.Get("cookie") != "":
		return JSONPointer("cookie", TagName(f.Tag.Get("cookie")))
	case f.Tag.Get("form") != "":
		return JSONPointer("form", TagName(f.Tag.Get("form")))
	case f.Tag.Get("file") != "":
		return JSONPointer("file", TagName(f.Tag.Get("file")))
	case f.Tag.Get("files") != "":
		return JSONPointer("files", TagName(f.Tag.Get("files")))
	case f.Tag.Get("body") != "" || f.Name == "Body":
		return ""
	default:
		if name, ok := JSONFieldName(f); ok {
			return escapeJSONPointerToken(name)
		}
	}
	return escapeJSONPointerToken(f.Name)
}
