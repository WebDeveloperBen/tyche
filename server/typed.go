package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/webdeveloperben/tyche/server/openapi"
	"github.com/webdeveloperben/tyche/server/validation"
)

type Operation struct {
	OperationID         string
	Method              string
	Path                string
	Summary             string
	Description         string
	Tags                []string
	DefaultStatus       int
	Deprecated          bool
	SkipValidateRequest bool
}

type TypedHandler[I, O any] func(context.Context, *I) (*O, error)

type RegisteredOperation struct {
	Method      string
	Path        string
	Summary     string
	Description string
	OperationID string
	Tags        []string
	InputType   reflect.Type
	OutputType  reflect.Type
}

func (r *Router) OpenAPI() *openapi.OpenAPI {
	if r.openapiDoc == nil {
		r.openapiDoc = openapi.NewOpenAPI("API", "1.0.0")
	}
	return r.openapiDoc
}

func (r *Router) SchemaRegistry() *openapi.Registry {
	if r.schemaRegistry == nil {
		r.schemaRegistry = openapi.NewRegistry("#/components/schemas")
	}
	return r.schemaRegistry
}

func (r *Router) RegisteredOperations() []RegisteredOperation {
	return append([]RegisteredOperation(nil), r.operations...)
}

func Register[I, O any](grp *Group, op Operation, handler TypedHandler[I, O]) {
	if err := RegisterE(grp, op, handler); err != nil {
		panic(err)
	}
}

func RegisterE[I, O any](grp *Group, op Operation, handler TypedHandler[I, O]) error {
	if grp == nil {
		return errors.New("group cannot be nil")
	}

	inputType := reflect.TypeFor[I]()
	outputType := reflect.TypeFor[O]()
	if !op.SkipValidateRequest {
		if _, err := validation.Struct(inputType); err != nil {
			return fmt.Errorf("invalid input validation for %s: %w", validation.IndirectType(inputType), err)
		}
	}

	if op.Method == "" {
		return errors.New("operation method is required")
	}
	if op.Path == "" {
		return errors.New("operation path is required")
	}
	if op.OperationID == "" {
		op.OperationID = fmt.Sprintf("%s-%s", strings.ToLower(op.Method), sanitizeOperationID(op.Path))
	}
	resolvedOp := op
	for _, existing := range grp.router.operations {
		if existing.OperationID == op.OperationID {
			return fmt.Errorf("duplicate operation ID: %s", op.OperationID)
		}
	}
	codec, hasCodec := generatedCodec(resolvedOp, inputType, outputType)
	if !hasCodec {
		if meta, ok := generatedRouteMeta(resolvedOp, inputType, outputType); ok && !meta.HasGeneratedCodec {
			return fmt.Errorf("servergen does not yet support a zero-overhead codec for operation: %s", op.OperationID)
		}
		return fmt.Errorf("missing generated codec for operation: %s (run servergen)", op.OperationID)
	}

	outputSpec := cloneOutputSpec(getOutputSpec[O]())

	if op.DefaultStatus != 0 && outputSpec.defaultStatus == http.StatusOK {
		outputSpec.defaultStatus = op.DefaultStatus
	}

	httpHandler := func(w http.ResponseWriter, req *http.Request) error {
		inputAny, err := codec.Parse(req)
		if err != nil {
			var validationErr *validation.Error
			if errors.As(err, &validationErr) {
				return validationErr
			}
			return NewHTTPError(http.StatusBadRequest, err.Error())
		}

		input, ok := inputAny.(*I)
		if !ok {
			return fmt.Errorf("generated codec %q returned wrong input type", op.OperationID)
		}

		out, err := handler(req.Context(), input)
		if err != nil {
			return err
		}

		return codec.Write(w, req, out)
	}

	if err := grp.HandleE(op.Method, op.Path, httpHandler); err != nil {
		return err
	}
	registerOpenAPIOperation(grp, op, inputType, outputType, outputSpec)
	return nil
}

func registerOpenAPIOperation(grp *Group, op Operation, inputType, outputType reflect.Type, outputSpec *outputSpec) {
	router := grp.router
	doc := router.OpenAPI()
	registry := router.SchemaRegistry()

	inputSchema := registry.Schema(inputType)
	outputSchema := registry.Schema(outputType)
	openAPIPath := ServerPathToOpenAPIPath(joinPath(grp.prefix, op.Path))

	docOp := &openapi.Operation{
		Summary:     op.Summary,
		Description: op.Description,
		OperationID: op.OperationID,
		Tags:        op.Tags,
		Parameters:  extractParameters(inputType, registry),
		RequestBody: extractRequestBody(inputType, registry),
		Responses:   extractResponses(outputSpec, registry),
		Deprecated:  op.Deprecated,
	}

	doc.AddOperation(op.Method, openAPIPath, docOp)

	inputName := validation.IndirectType(inputType).Name()
	if inputName == "" {
		inputName = fmt.Sprintf("InlineInput%s", sanitizeOperationID(openAPIPath))
	} else {
		inputName = SchemaComponentName(inputType)
	}
	doc.AddSchema(inputName, inputSchema)

	outputName := validation.IndirectType(outputType).Name()
	if outputName == "" {
		outputName = fmt.Sprintf("InlineOutput%s", sanitizeOperationID(openAPIPath))
	} else {
		outputName = SchemaComponentName(outputType)
	}
	doc.AddSchema(outputName, outputSchema)

	router.operations = append(router.operations, RegisteredOperation{
		Method:      op.Method,
		Path:        openAPIPath,
		Summary:     op.Summary,
		Description: op.Description,
		OperationID: op.OperationID,
		Tags:        op.Tags,
		InputType:   inputType,
		OutputType:  outputType,
	})
	router.invalidateOpenAPICache()
}

func extractParameters(t reflect.Type, registry *openapi.Registry) []*openapi.Parameter {
	t = validation.IndirectType(t)
	if t.Kind() != reflect.Struct {
		return nil
	}

	params := make([]*openapi.Parameter, 0)
	seen := make(map[string]bool)

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		var param *openapi.Parameter

		switch {
		case f.Tag.Get("path") != "":
			param = &openapi.Parameter{Name: validation.TagName(f.Tag.Get("path")), In: "path", Required: true}
		case f.Tag.Get("query") != "":
			param = &openapi.Parameter{Name: validation.TagName(f.Tag.Get("query")), In: "query", Required: validation.FieldRequired(f, "query")}
		case f.Tag.Get("header") != "":
			param = &openapi.Parameter{Name: validation.TagName(f.Tag.Get("header")), In: "header", Required: validation.FieldRequired(f, "header")}
		case f.Tag.Get("cookie") != "":
			param = &openapi.Parameter{Name: validation.TagName(f.Tag.Get("cookie")), In: "cookie", Required: validation.FieldRequired(f, "cookie")}
		}

		if param == nil {
			continue
		}

		key := param.In + ":" + param.Name
		if seen[key] {
			continue
		}

		param.Schema = openapi.CloneSchema(registry.Schema(f.Type))
		openapi.ApplyFieldSchemaMetadata(param.Schema, f)
		params = append(params, param)
		seen[key] = true
	}

	return params
}

func extractRequestBody(t reflect.Type, registry *openapi.Registry) *openapi.RequestBody {
	spec := inputSpecForType(t)
	if spec.bodyMode == bodyModeNone {
		return nil
	}

	return &openapi.RequestBody{
		Required: spec.bodyRequired,
		Content: map[string]*openapi.MediaType{
			"application/json": {Schema: bodySchemaForSpec(spec, registry)},
		},
	}
}

func extractResponses(spec *outputSpec, registry *openapi.Registry) map[string]*openapi.Response {
	statusCode := strconv.Itoa(spec.defaultStatus)
	resp := map[string]*openapi.Response{
		statusCode: {
			Description: "Successful response",
			Headers:     responseHeadersForSpec(spec, registry),
			Content: map[string]*openapi.MediaType{
				"application/json": {Schema: &openapi.Schema{
					Type: "object",
					Properties: map[string]*openapi.Schema{
						"data": bodySchemaForOutputSpec(spec, registry),
					},
				}},
			},
		},
		"default": {
			Description: "Error response",
			Content: map[string]*openapi.MediaType{
				"application/problem+json": {Schema: &openapi.Schema{
					Type: "object",
					Properties: map[string]*openapi.Schema{
						"type":   {Type: "string"},
						"status": {Type: "integer"},
						"title":  {Type: "string"},
						"detail": {Type: "string"},
					},
				}},
			},
		},
	}

	if spec.noBody || statusMustNotHaveBody(spec.defaultStatus) {
		resp[statusCode].Content = nil
	}

	return resp
}

func ParseRequest[I any](req *http.Request) (*I, error) {
	spec := getInputSpec[I]()
	value := reflect.New(spec.typ)
	target := value.Elem()
	validationErr := &validation.Error{}

	for _, binding := range spec.bindings {
		raw, ok, err := binding.read(req)
		if err != nil {
			return nil, err
		}
		if !ok {
			if binding.required {
				validationErr.AddRequired(validation.JSONPointer(binding.source, binding.name))
			}
			continue
		}

		if err := binding.set(target.Field(binding.index), raw); err != nil {
			validationErr.AddInvalidType(validation.JSONPointer(binding.source, binding.name))
		}
	}
	if !validationErr.Empty() {
		return nil, validationErr
	}

	if spec.bodyMode != bodyModeNone {
		if err := decodeRequestBody(req, target, spec); err != nil {
			return nil, err
		}
	}

	if spec.validationSpec.Load() == nil {
		specPtr, err := validation.Struct(spec.typ)
		if err != nil {
			return nil, err
		}
		spec.validationSpec.Store(specPtr)
	}
	if err := validation.ValidateStructValue(target, spec.validationSpec.Load(), "request"); err != nil {
		return nil, err
	}

	return value.Interface().(*I), nil
}

func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(v)
}

// DataResponse is the standard envelope for all successful API responses.
type DataResponse struct {
	Data any `json:"data"`
}

// WriteSuccess writes a successful JSON response wrapped in the standard DataResponse envelope.
func WriteSuccess(w http.ResponseWriter, status int, data any) error {
	return WriteJSON(w, status, DataResponse{Data: data})
}

type inputBinding struct {
	index    int
	name     string
	location string
	source   string
	required bool
	read     func(*http.Request) (string, bool, error)
	set      func(reflect.Value, string) error
}

type bodyBindingMode uint8

const (
	bodyModeNone bodyBindingMode = iota
	bodyModeStruct
	bodyModeField
)

type RequiredJSONField struct {
	Pointer string
	Path    []string
}

type inputSpec struct {
	typ                reflect.Type
	bindings           []inputBinding
	bodyMode           bodyBindingMode
	bodyIndex          int
	bodyType           reflect.Type
	bodyRequired       bool
	requiredBodyFields []RequiredJSONField
	validationSpec     atomic.Pointer[validation.StructSpec]
}

var inputSpecCache sync.Map

func getInputSpec[I any]() *inputSpec {
	t := validation.IndirectType(reflect.TypeFor[I]())
	if spec, ok := inputSpecCache.Load(t); ok {
		return spec.(*inputSpec)
	}
	spec := inputSpecForType(t)
	actual, _ := inputSpecCache.LoadOrStore(t, spec)
	return actual.(*inputSpec)
}

func inputSpecForType(t reflect.Type) *inputSpec {
	t = validation.IndirectType(t)
	spec := &inputSpec{
		typ:       t,
		bindings:  make([]inputBinding, 0, max(0, t.NumField())),
		bodyIndex: -1,
	}

	if t.Kind() != reflect.Struct {
		return spec
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}

		switch {
		case f.Tag.Get("path") != "":
			name := validation.TagName(f.Tag.Get("path"))
			spec.bindings = append(spec.bindings, inputBinding{
				index:    i,
				name:     name,
				location: "path parameter",
				source:   "path",
				required: true,
				read: func(req *http.Request) (string, bool, error) {
					if val := Param(req, name); val != "" {
						return val, true, nil
					}
					return "", false, nil
				},
				set: setStringValue,
			})
		case f.Tag.Get("query") != "":
			name := validation.TagName(f.Tag.Get("query"))
			spec.bindings = append(spec.bindings, inputBinding{
				index:    i,
				name:     name,
				location: "query parameter",
				source:   "query",
				required: validation.FieldRequired(f, "query"),
				read: func(req *http.Request) (string, bool, error) {
					values, ok := req.URL.Query()[name]
					if !ok || len(values) == 0 {
						return "", false, nil
					}
					return values[0], true, nil
				},
				set: setStringValue,
			})
		case f.Tag.Get("header") != "":
			name := validation.TagName(f.Tag.Get("header"))
			spec.bindings = append(spec.bindings, inputBinding{
				index:    i,
				name:     name,
				location: "header",
				source:   "header",
				required: validation.FieldRequired(f, "header"),
				read: func(req *http.Request) (string, bool, error) {
					values := req.Header.Values(name)
					if len(values) == 0 {
						return "", false, nil
					}
					return values[0], true, nil
				},
				set: setStringValue,
			})
		case f.Tag.Get("cookie") != "":
			name := validation.TagName(f.Tag.Get("cookie"))
			spec.bindings = append(spec.bindings, inputBinding{
				index:    i,
				name:     name,
				location: "cookie",
				source:   "cookie",
				required: validation.FieldRequired(f, "cookie"),
				read: func(req *http.Request) (string, bool, error) {
					cookie, err := req.Cookie(name)
					if errors.Is(err, http.ErrNoCookie) {
						return "", false, nil
					}
					if err != nil {
						return "", false, err
					}
					return cookie.Value, true, nil
				},
				set: setStringValue,
			})
		case f.Tag.Get("body") != "" || f.Name == "Body":
			spec.bodyMode = bodyModeField
			spec.bodyIndex = i
			spec.bodyType = validation.IndirectType(f.Type)
			spec.bodyRequired = validation.FieldRequired(f, "json")
			spec.requiredBodyFields = RequiredJSONFields(spec.bodyType, nil, nil)
		case validation.IsJSONBodyField(f):
			if spec.bodyMode == bodyModeNone {
				spec.bodyMode = bodyModeStruct
				spec.bodyType = t
				spec.bodyRequired = hasRequiredJSONFields(t)
				spec.requiredBodyFields = RequiredJSONFields(t, nil, nil)
			}
		}
	}

	return spec
}

func decodeRequestBody(req *http.Request, target reflect.Value, spec *inputSpec) error {
	bodyBytes, err := readJSONBody(req)
	if err != nil {
		return err
	}
	if len(bodyBytes) == 0 {
		if spec.bodyRequired {
			validationErr := &validation.Error{}
			validationErr.AddRequired("")
			return validationErr
		}
		return nil
	}
	if err := ValidateRequiredJSONFields(bodyBytes, spec.requiredBodyFields); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.DisallowUnknownFields()

	switch spec.bodyMode {
	case bodyModeStruct:
		if err := decoder.Decode(target.Addr().Interface()); err != nil {
			return fmt.Errorf("failed to decode body: %w", err)
		}
	case bodyModeField:
		field := target.Field(spec.bodyIndex)
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			if err := decoder.Decode(field.Interface()); err != nil {
				return fmt.Errorf("failed to decode body: %w", err)
			}
		} else {
			if err := decoder.Decode(field.Addr().Interface()); err != nil {
				return fmt.Errorf("failed to decode body: %w", err)
			}
		}
	}

	return ensureSingleJSONValue(decoder)
}

func setStringValue(v reflect.Value, val string) error {
	if v.Kind() == reflect.Pointer {
		if val == "" {
			return nil
		}
		v.Set(reflect.New(v.Type().Elem()))
		return setStringValue(v.Elem(), val)
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(val)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(i)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(i)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return err
		}
		v.SetFloat(f)
	case reflect.Bool:
		b, err := strconv.ParseBool(val)
		if err != nil {
			return err
		}
		v.SetBool(b)
	default:
		return fmt.Errorf("unsupported field type %s", v.Type())
	}
	return nil
}

func ReadRequestJSONBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	if err := validateJSONContentType(req.Header.Get("Content-Type")); err != nil {
		return nil, err
	}
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, fmt.Errorf("request body too large: limit is %d bytes", maxBytesErr.Limit)
		}
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	if isBlankJSONBody(bodyBytes) {
		return nil, nil
	}
	return bodyBytes, nil
}

func ReadRequestJSONBodyFast(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	if err := validateJSONContentType(req.Header.Get("Content-Type")); err != nil {
		return nil, err
	}
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, fmt.Errorf("request body too large: limit is %d bytes", maxBytesErr.Limit)
		}
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	if isBlankJSONBody(bodyBytes) {
		return nil, nil
	}
	return bodyBytes, nil
}

func DecodeRequestJSONBodyFast(req *http.Request, dst any) error {
	if req.Body == nil {
		return nil
	}
	if err := validateJSONContentType(req.Header.Get("Content-Type")); err != nil {
		return err
	}
	buf := requestBodyBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= 1024*1024 {
			requestBodyBufPool.Put(buf)
		}
	}()
	if _, err := buf.ReadFrom(req.Body); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return fmt.Errorf("request body too large: limit is %d bytes", maxBytesErr.Limit)
		}
		return fmt.Errorf("failed to read body: %w", err)
	}
	bodyBytes := buf.Bytes()
	if isBlankJSONBody(bodyBytes) {
		return nil
	}
	if err := json.Unmarshal(bodyBytes, dst); err != nil {
		return fmt.Errorf("failed to decode body: %w", err)
	}
	return nil
}

func DecodeRequestJSONBodyStrictFast(req *http.Request, dst any, bodyRequired bool, required []RequiredJSONField) error {
	if req.Body == nil {
		if bodyRequired {
			validationErr := &validation.Error{}
			validationErr.AddRequired("")
			return validationErr
		}
		return nil
	}
	if err := validateJSONContentType(req.Header.Get("Content-Type")); err != nil {
		return err
	}
	buf := requestBodyBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= 1024*1024 {
			requestBodyBufPool.Put(buf)
		}
	}()
	if _, err := buf.ReadFrom(req.Body); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return fmt.Errorf("request body too large: limit is %d bytes", maxBytesErr.Limit)
		}
		return fmt.Errorf("failed to read body: %w", err)
	}
	bodyBytes := buf.Bytes()
	if isBlankJSONBody(bodyBytes) {
		if bodyRequired {
			validationErr := &validation.Error{}
			validationErr.AddRequired("")
			return validationErr
		}
		return nil
	}
	if err := ValidateRequiredJSONFields(bodyBytes, required); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("failed to decode body: %w", err)
	}
	return ensureSingleJSONValue(decoder)
}

func readJSONBody(req *http.Request) ([]byte, error) {
	return ReadRequestJSONBody(req)
}

func validateJSONContentType(contentType string) error {
	if contentType == "" {
		return nil
	}
	mediaType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	if mediaType != "" && mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return fmt.Errorf("unsupported content type %q", mediaType)
	}
	return nil
}

func ValidateJSONContentType(contentType string) error {
	return validateJSONContentType(contentType)
}

func isBlankJSONBody(body []byte) bool {
	for _, b := range body {
		switch b {
		case ' ', '\t', '\r', '\n':
			continue
		default:
			return false
		}
	}
	return true
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return fmt.Errorf("failed to decode body: %w", err)
	}
	return nil
}

func EnsureSingleJSONValue(decoder *json.Decoder) error {
	return ensureSingleJSONValue(decoder)
}

func ValidateRequiredJSONFields(body []byte, required []RequiredJSONField) error {
	if len(required) == 0 {
		return nil
	}
	validationErr := &validation.Error{}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("failed to decode body: %w", err)
	}
	for _, field := range required {
		if !jsonPathPresent(payload, field.Path) {
			validationErr.AddRequired(field.Pointer)
		}
	}
	if validationErr.Empty() {
		return nil
	}
	return validationErr
}

func jsonPathPresent(value any, path []string) bool {
	if len(path) == 0 {
		return true
	}
	part := path[0]
	if part == "*" {
		items, ok := value.([]any)
		if !ok {
			return false
		}
		for _, item := range items {
			if !jsonPathPresent(item, path[1:]) {
				return false
			}
		}
		return true
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	next, ok := obj[part]
	if !ok {
		return false
	}
	return jsonPathPresent(next, path[1:])
}

var requestBodyBufPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

type outputHeaderField struct {
	index    int
	name     string
	required bool
	get      func(reflect.Value) (string, bool, error)
}

type outputSpec struct {
	typ           reflect.Type
	bodyIndex     int
	statusIndex   int
	defaultStatus int
	headers       []outputHeaderField
	noBody        bool
	bodyValue     func(reflect.Value) any
}

var outputSpecCache sync.Map

func getOutputSpec[O any]() *outputSpec {
	t := validation.IndirectType(reflect.TypeFor[O]())
	if spec, ok := outputSpecCache.Load(t); ok {
		return spec.(*outputSpec)
	}
	spec := outputSpecForType(t)
	actual, _ := outputSpecCache.LoadOrStore(t, spec)
	return actual.(*outputSpec)
}

func outputSpecForType(t reflect.Type) *outputSpec {
	t = validation.IndirectType(t)
	spec := &outputSpec{
		typ:           t,
		bodyIndex:     -1,
		statusIndex:   -1,
		defaultStatus: http.StatusOK,
		bodyValue: func(value reflect.Value) any {
			if !value.IsValid() {
				return nil
			}
			return value.Interface()
		},
	}
	bodyIndex := -1
	statusIndex := -1
	if t.Kind() != reflect.Struct {
		return spec
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		switch {
		case f.Tag.Get("body") != "" || f.Name == "Body":
			bodyIndex = i
		case f.Tag.Get("header") != "":
			index := i
			spec.headers = append(spec.headers, outputHeaderField{
				index:    index,
				name:     validation.TagName(f.Tag.Get("header")),
				required: validation.FieldRequired(f, "header"),
				get: func(value reflect.Value) (string, bool, error) {
					field := value.Field(index)
					if isZeroValue(field) {
						return "", false, nil
					}
					headerValue, err := responseHeaderValue(field)
					if err != nil {
						return "", false, err
					}
					return headerValue, true, nil
				},
			})
		case f.Tag.Get("status") != "" || f.Name == "Status":
			statusIndex = i
			if tag := f.Tag.Get("status"); tag != "" {
				if status, err := strconv.Atoi(tag); err == nil {
					spec.defaultStatus = status
				}
			}
		}
	}
	if statusIndex >= 0 {
		spec.statusIndex = statusIndex
	}
	if bodyIndex >= 0 {
		spec.bodyIndex = bodyIndex
		spec.bodyValue = func(value reflect.Value) any {
			if !value.IsValid() || value.Kind() != reflect.Struct {
				return nil
			}
			field := value.Field(bodyIndex)
			if field.Kind() == reflect.Pointer && field.IsNil() {
				return nil
			}
			return field.Interface()
		}
		bodyField := validation.IndirectType(t.Field(bodyIndex).Type)
		if bodyField.Kind() == reflect.Struct && bodyField.NumField() == 0 {
			spec.noBody = true
		}
	}
	return spec
}

func responseHeaderValue(v reflect.Value) (string, error) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return "", nil
		}
		return responseHeaderValue(v.Elem())
	}
	switch v.Kind() {
	case reflect.String:
		return v.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), nil
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), nil
	default:
		return "", fmt.Errorf("unsupported header field type %s", v.Type())
	}
}

func responseHeadersForSpec(spec *outputSpec, registry *openapi.Registry) map[string]*openapi.Parameter {
	if len(spec.headers) == 0 || spec.typ.Kind() != reflect.Struct {
		return nil
	}
	headers := make(map[string]*openapi.Parameter, len(spec.headers))
	for _, header := range spec.headers {
		field := spec.typ.Field(header.index)
		headers[header.name] = &openapi.Parameter{
			Name:        header.name,
			In:          "header",
			Description: field.Tag.Get("doc"),
			Required:    header.required,
			Schema:      openapi.CloneSchema(registry.Schema(field.Type)),
		}
	}
	return headers
}

func bodySchemaForSpec(spec *inputSpec, registry *openapi.Registry) *openapi.Schema {
	switch spec.bodyMode {
	case bodyModeStruct:
		bodySchema := &openapi.Schema{Type: "object", Properties: map[string]*openapi.Schema{}}
		for i := 0; i < spec.typ.NumField(); i++ {
			f := spec.typ.Field(i)
			if !f.IsExported() || hasParamTag(f) {
				continue
			}
			jsonName, ok := jsonFieldName(f)
			if !ok {
				continue
			}
			fieldSchema := openapi.CloneSchema(registry.Schema(f.Type))
			openapi.ApplyFieldSchemaMetadata(fieldSchema, f)
			bodySchema.Properties[jsonName] = fieldSchema
			if fieldRequired(f, "json") {
				bodySchema.Required = append(bodySchema.Required, jsonName)
			}
		}
		if len(bodySchema.Required) == 0 {
			bodySchema.Required = nil
		}
		return bodySchema
	case bodyModeField:
		field := spec.typ.Field(spec.bodyIndex)
		return openapi.CloneSchema(registry.Schema(field.Type))
	default:
		return nil
	}
}

func bodySchemaForOutputSpec(spec *outputSpec, registry *openapi.Registry) *openapi.Schema {
	if spec.typ.Kind() != reflect.Struct || spec.bodyIndex < 0 {
		return openapi.CloneSchema(registry.Schema(spec.typ))
	}
	field := spec.typ.Field(spec.bodyIndex)
	return openapi.CloneSchema(registry.Schema(field.Type))
}

func indirectType(t reflect.Type) reflect.Type {
	return validation.IndirectType(t)
}

func fieldRequired(f reflect.StructField, tagKey string) bool {
	return validation.FieldRequired(f, tagKey)
}

func hasParamTag(f reflect.StructField) bool {
	return validation.HasParamTag(f)
}

func jsonFieldName(f reflect.StructField) (string, bool) {
	return validation.JSONFieldName(f)
}

func hasRequiredJSONFields(t reflect.Type) bool {
	return len(RequiredJSONFields(t, nil, nil)) > 0
}

func RequiredJSONFields(t reflect.Type, pointerPrefix, pathPrefix []string) []RequiredJSONField {
	t = validation.IndirectType(t)
	if t.Kind() != reflect.Struct {
		return nil
	}
	fields := make([]RequiredJSONField, 0)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || hasParamTag(f) {
			continue
		}
		if f.Tag.Get("body") != "" || f.Name == "Body" {
			fields = append(fields, RequiredJSONFields(f.Type, pointerPrefix, pathPrefix)...)
			continue
		}
		name, ok := jsonFieldName(f)
		if !ok {
			continue
		}
		pointerPath := append(append([]string(nil), pointerPrefix...), name)
		path := append(append([]string(nil), pathPrefix...), name)
		required := validation.FieldRequired(f, "json")
		if required {
			fields = append(fields, RequiredJSONField{Pointer: validation.JSONPointer(pointerPath...), Path: path})
		}
		fieldType := validation.IndirectType(f.Type)
		switch fieldType.Kind() {
		case reflect.Struct:
			if required && !isScalarStruct(fieldType) {
				fields = append(fields, RequiredJSONFields(f.Type, pointerPath, path)...)
			}
		case reflect.Slice, reflect.Array:
			elemType := validation.IndirectType(fieldType.Elem())
			if required && elemType.Kind() == reflect.Struct && !isScalarStruct(elemType) {
				fields = append(fields, RequiredJSONFields(elemType, pointerPath, append(path, "*"))...)
			}
		}
	}
	return fields
}

func isScalarStruct(t reflect.Type) bool {
	return t.PkgPath() == "time" && t.Name() == "Time"
}

func isZeroValue(v reflect.Value) bool {
	return !v.IsValid() || v.IsZero()
}

func ServerPathToOpenAPIPath(path string) string {
	if path == "" || (!strings.Contains(path, ":") && !strings.Contains(path, "*")) {
		return path
	}
	parts := SplitRouteFast(path)
	if len(parts) == 0 {
		return path
	}
	var b strings.Builder
	for _, part := range parts {
		b.WriteByte('/')
		if len(part) > 0 && part[0] == ':' {
			b.WriteByte('{')
			b.WriteString(part[1:])
			b.WriteByte('}')
			continue
		}
		if len(part) > 0 && part[0] == '*' {
			name := part[1:]
			if name == "" {
				name = "wildcard"
			}
			b.WriteByte('{')
			b.WriteString(name)
			b.WriteByte('}')
			continue
		}
		b.WriteString(part)
	}
	if b.Len() == 0 {
		return "/"
	}
	return b.String()
}

func sanitizeOperationID(path string) string {
	path = strings.TrimPrefix(path, "/")
	return strings.NewReplacer("/", "_", ":", "", "{", "", "}", "", "*", "", "-", "_").Replace(path)
}

func SchemaComponentName(t reflect.Type) string {
	base := indirectType(t)
	if base == nil {
		return ""
	}
	if base.PkgPath() == "" || base.Name() == "" {
		return base.String()
	}
	pkg := strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(base.PkgPath())
	return pkg + "_" + base.Name()
}

func cloneOutputSpec(spec *outputSpec) *outputSpec {
	if spec == nil {
		return nil
	}
	clone := *spec
	if spec.headers != nil {
		clone.headers = append([]outputHeaderField(nil), spec.headers...)
	}
	return &clone
}

func statusMustNotHaveBody(status int) bool {
	return (status >= 100 && status < 200) || status == http.StatusNoContent || status == http.StatusNotModified
}
