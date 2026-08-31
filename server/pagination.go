package server

import (
	"net/http"
	"reflect"
	"sync"

	"github.com/webdeveloperben/tyche/pagination"
)

var (
	paginationParamsType    = reflect.TypeFor[pagination.Params]()
	paginationBindingsCache sync.Map
)

type paginationBinding struct {
	index []int
}

func parseWithPaginationPolicy[I any](req *http.Request, parse func(*http.Request) (any, error), config pagination.Config) (any, error) {
	bindings := paginationBindings(reflect.TypeFor[I]())
	if len(bindings) == 0 {
		return parse(req)
	}

	params, err := config.Parse(req.URL.Query())
	if err != nil {
		return nil, err
	}
	value, err := parse(req)
	if err != nil {
		return nil, err
	}
	input, ok := value.(*I)
	if !ok {
		return nil, errWrongGeneratedInputType
	}
	root := reflect.ValueOf(input).Elem()
	for _, binding := range bindings {
		field := fieldByIndex(root, binding.index)
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field = field.Elem()
		}
		field.Set(reflect.ValueOf(params))
	}
	return value, nil
}

func paginationBindings(inputType reflect.Type) []paginationBinding {
	inputType = indirectReflectType(inputType)
	if cached, ok := paginationBindingsCache.Load(inputType); ok {
		return cached.([]paginationBinding)
	}
	var bindings []paginationBinding
	collectPaginationBindings(inputType, nil, make(map[reflect.Type]bool), &bindings)
	actual, _ := paginationBindingsCache.LoadOrStore(inputType, bindings)
	return actual.([]paginationBinding)
}

func collectPaginationBindings(inputType reflect.Type, prefix []int, active map[reflect.Type]bool, bindings *[]paginationBinding) {
	inputType = indirectReflectType(inputType)
	if inputType == paginationParamsType {
		index := append([]int(nil), prefix...)
		*bindings = append(*bindings, paginationBinding{index: index})
		return
	}
	if inputType.Kind() != reflect.Struct || active[inputType] {
		return
	}
	active[inputType] = true
	defer delete(active, inputType)

	for i := 0; i < inputType.NumField(); i++ {
		field := inputType.Field(i)
		if field.PkgPath != "" {
			continue
		}
		index := append(append([]int(nil), prefix...), i)
		fieldType := indirectReflectType(field.Type)
		if fieldType == paginationParamsType {
			*bindings = append(*bindings, paginationBinding{index: index})
			continue
		}
		if field.Anonymous && fieldType.Kind() == reflect.Struct && shouldTraversePaginationField(field) {
			collectPaginationBindings(fieldType, index, active, bindings)
		}
	}
}

func shouldTraversePaginationField(field reflect.StructField) bool {
	for _, key := range []string{"path", "query", "header", "cookie", "form", "file", "files", "body", "json"} {
		if field.Tag.Get(key) != "" {
			return false
		}
	}
	return field.Name != "Body"
}

func indirectReflectType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

var errWrongGeneratedInputType = &wrongGeneratedInputTypeError{}

type wrongGeneratedInputTypeError struct{}

func (*wrongGeneratedInputTypeError) Error() string {
	return "pagination policy received the wrong generated input type"
}

func validatePaginationConfig(inputType reflect.Type, config pagination.Config) error {
	if len(paginationBindings(inputType)) == 0 {
		return nil
	}
	_, err := config.Normalize(pagination.Params{})
	return err
}
