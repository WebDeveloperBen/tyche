package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
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
	OperationID string
	Method      string
	Path        string
	Summary     string
	Description string
	Tags        []string
	// Security lists the security requirements for this operation. Each entry
	// is a set of scheme names (referencing schemes registered via
	// [API.AddSecurityScheme]) that must all be satisfied; multiple entries
	// are alternatives (logical OR). The value for each scheme is the list of
	// required OAuth2 scopes, or an empty slice for non-OAuth2 schemes. Use
	// [SecurityRequirement] to build entries.
	Security            []SecurityRequirement
	DefaultStatus       int
	Deprecated          bool
	SkipValidateRequest bool
}

// SecurityRequirement maps security scheme names to the scopes they require for
// an operation. An empty (or nil) SecurityRequirement documents that the
// operation may be called without authentication.
type SecurityRequirement = map[string][]string

type TypedHandler[I, O any] func(context.Context, *I) (*O, error)

type RegisteredOperation struct {
	InputType   reflect.Type
	OutputType  reflect.Type
	Method      string
	Path        string
	Summary     string
	Description string
	OperationID string
	Tags        []string
}

func Register[I, O any](grp RouteTarget, op Operation, handler TypedHandler[I, O], opts ...RouteOption) {
	if err := RegisterE(grp, op, handler, opts...); err != nil {
		panic(err)
	}
}

func RegisterE[I, O any](grp RouteTarget, op Operation, handler TypedHandler[I, O], opts ...RouteOption) error {
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
	for _, existing := range grp.apiOperations() {
		if existing.OperationID == op.OperationID {
			return fmt.Errorf("duplicate operation ID: %s", op.OperationID)
		}
	}
	outputSpec := cloneOutputSpec(getOutputSpec[O]())

	if op.DefaultStatus != 0 && outputSpec.defaultStatus == http.StatusOK {
		outputSpec.defaultStatus = op.DefaultStatus
	}

	ro := resolveRouteOptions(opts)
	apiCodecs := grp.apiCodecs()
	requestCodecs, err := codecsForMediaTypes(apiCodecs, ro.requestContentTypes)
	if err != nil {
		return err
	}
	responseCodecs, err := codecsForMediaTypes(apiCodecs, ro.responseContentTypes)
	if err != nil {
		return err
	}
	codec, hasCodec := generatedCodec(resolvedOp, inputType, outputType)
	if hasCodec && (hasNonJSONCodec(requestCodecs) || hasNonJSONCodec(responseCodecs)) &&
		(codec.ParseWithCodecs == nil || codec.WriteWithCodecs == nil) {
		hasCodec = false
	}
	if !hasCodec {
		// No generated codec (servergen not run, or this route shape isn't yet
		// supported by codegen): fall back to the reflection binder so the route
		// works during development. Generated codecs remain a zero-reflection
		// optimization for production — run servergen to get them.
		codec = GeneratedRouteCodec{
			Parse: func(req *http.Request) (any, error) {
				in, err := parseRequestWithCodecs[I](req, requestCodecs)
				if err != nil {
					return nil, err
				}
				return in, nil
			},
			Write: func(w http.ResponseWriter, req *http.Request, value any) error {
				return writeReflectionResponse(w, req, value, outputSpec, responseCodecs)
			},
		}
	}
	parse := codec.Parse
	if codec.ParseWithCodecs != nil {
		parse = func(req *http.Request) (any, error) {
			return codec.ParseWithCodecs(req, requestCodecs)
		}
	}
	write := codec.Write
	if codec.WriteWithCodecs != nil {
		write = func(w http.ResponseWriter, req *http.Request, value any) error {
			return codec.WriteWithCodecs(w, req, value, responseCodecs)
		}
	}
	if parse == nil || write == nil {
		return fmt.Errorf("generated codec %q is incomplete", op.OperationID)
	}

	httpHandler := func(w http.ResponseWriter, req *http.Request) error {
		inputAny, err := parse(req)
		if err != nil {
			var validationErr *validation.Error
			if errors.As(err, &validationErr) {
				return validationErr
			}
			var httpErr HTTPError
			if errors.As(err, &httpErr) {
				return httpErr
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

		return write(w, req, out)
	}

	if err := grp.handleRoute(op.Method, op.Path, httpHandler, ro); err != nil {
		return err
	}
	registerOpenAPIOperation(grp, op, inputType, outputType, outputSpec, requestCodecs, responseCodecs)
	return nil
}

func registerOpenAPIOperation(grp RouteTarget, op Operation, inputType, outputType reflect.Type, outputSpec *outputSpec, requestCodecs, responseCodecs []Codec) {
	doc := grp.apiDoc()
	registry := grp.apiSchemaRegistry()

	inputSchema := registry.Schema(inputType)
	outputSchema := registry.Schema(outputType)
	openAPIPath := ServerPathToOpenAPIPath(joinPath(grp.groupPrefix(), op.Path))

	docOp := &openapi.Operation{
		Summary:     op.Summary,
		Description: op.Description,
		OperationID: op.OperationID,
		Tags:        op.Tags,
		Parameters:  extractParameters(inputType, registry),
		RequestBody: extractRequestBody(inputType, registry, requestCodecs),
		Responses:   extractResponses(outputSpec, registry, responseCodecs),
		Deprecated:  op.Deprecated,
		Security:    normalizeSecurityRequirements(op.Security),
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

	grp.recordOperation(RegisteredOperation{
		Method:      op.Method,
		Path:        openAPIPath,
		Summary:     op.Summary,
		Description: op.Description,
		OperationID: op.OperationID,
		Tags:        op.Tags,
		InputType:   inputType,
		OutputType:  outputType,
	})
	grp.invalidateOpenAPI()
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

func extractRequestBody(t reflect.Type, registry *openapi.Registry, codecs []Codec) *openapi.RequestBody {
	spec := inputSpecForType(t)
	if spec.bodyMode == bodyModeNone {
		return nil
	}
	mediaType := "application/json"
	if spec.bodyMode == bodyModeMultipart {
		mediaType = "multipart/form-data"
	}
	content := map[string]*openapi.MediaType{
		mediaType: {Schema: bodySchemaForSpec(spec, registry)},
	}
	if spec.bodyMode != bodyModeMultipart {
		content = codecContentMap(codecs, bodySchemaForSpec(spec, registry))
	}

	return &openapi.RequestBody{
		Required: spec.bodyRequired,
		Content:  content,
	}
}

func extractResponses(spec *outputSpec, registry *openapi.Registry, codecs []Codec) map[string]*openapi.Response {
	statusCode := strconv.Itoa(spec.defaultStatus)
	resp := map[string]*openapi.Response{
		statusCode: {
			Description: "Successful response",
			Headers:     responseHeadersForSpec(spec, registry),
			Content: codecContentMap(codecs, &openapi.Schema{
				Type: "object",
				Properties: map[string]*openapi.Schema{
					"data": bodySchemaForOutputSpec(spec, registry),
				},
			}),
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

func codecContentMap(codecs []Codec, schema *openapi.Schema) map[string]*openapi.MediaType {
	content := make(map[string]*openapi.MediaType, len(codecs))
	for _, codec := range codecs {
		if codec == nil {
			continue
		}
		mediaType := strings.TrimSpace(codec.MediaType())
		if mediaType == "" {
			continue
		}
		content[mediaType] = &openapi.MediaType{Schema: openapi.CloneSchema(schema)}
	}
	if len(content) == 0 {
		content["application/json"] = &openapi.MediaType{Schema: openapi.CloneSchema(schema)}
	}
	return content
}

func ParseRequest[I any](req *http.Request) (*I, error) {
	return parseRequestWithCodecs[I](req, []Codec{JSONCodec{}})
}

func ParseRequestWithCodecs[I any](req *http.Request, codecs []Codec) (*I, error) {
	return parseRequestWithCodecs[I](req, codecs)
}

func parseRequestWithCodecs[I any](req *http.Request, codecs []Codec) (*I, error) {
	spec := getInputSpec[I]()
	value := reflect.New(spec.typ)
	target := value.Elem()
	validationErr := &validation.Error{}

	for _, binding := range spec.bindings {
		if binding.readFile != nil {
			file, ok, err := binding.readFile(req)
			if err != nil {
				return nil, err
			}
			if !ok {
				if binding.required {
					validationErr.AddRequired(validation.JSONPointer(binding.source, binding.name))
				}
				continue
			}
			if binding.setFile != nil {
				if err := binding.setFile(target.Field(binding.index), file); err != nil {
					validationErr.AddInvalidType(validation.JSONPointer(binding.source, binding.name))
				}
			}
		} else if binding.readFiles != nil {
			files, ok, err := binding.readFiles(req)
			if err != nil {
				return nil, err
			}
			if !ok {
				if binding.required {
					validationErr.AddRequired(validation.JSONPointer(binding.source, binding.name))
				}
				continue
			}
			if binding.setFiles != nil {
				if err := binding.setFiles(target.Field(binding.index), files); err != nil {
					validationErr.AddInvalidType(validation.JSONPointer(binding.source, binding.name))
				}
			}
		} else if binding.readSlice != nil {
			values, ok, err := binding.readSlice(req)
			if err != nil {
				return nil, err
			}
			if !ok {
				if binding.required {
					validationErr.AddRequired(validation.JSONPointer(binding.source, binding.name))
				}
				continue
			}
			if binding.setSlice != nil {
				if err := binding.setSlice(target.Field(binding.index), values); err != nil {
					validationErr.AddInvalidType(validation.JSONPointer(binding.source, binding.name))
				}
			}
		} else if binding.read != nil {
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
	}
	if !validationErr.Empty() {
		return nil, validationErr
	}

	if spec.bodyMode != bodyModeNone && spec.bodyMode != bodyModeMultipart {
		if err := decodeRequestBody(req, target, spec, codecs); err != nil {
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

// writeReflectionResponse writes a typed output using the output spec derived
// by reflection, mirroring what a generated codec produces: it applies the
// status (from a Status field or the default), sets response headers, and wraps
// the body in the {"data": …} success envelope. It is the write half of the
// no-codegen fallback used by RegisterE.
func writeReflectionResponse(w http.ResponseWriter, req *http.Request, value any, spec *outputSpec, codecs []Codec) error {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() || (rv.Kind() == reflect.Pointer && rv.IsNil()) {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	outVal := reflect.Indirect(rv)

	status := spec.defaultStatus
	if spec.statusIndex >= 0 && outVal.Kind() == reflect.Struct {
		f := outVal.Field(spec.statusIndex)
		if f.CanInt() {
			if s := int(f.Int()); s != 0 {
				status = s
			}
		}
	}

	for _, h := range spec.headers {
		v, ok, err := h.get(outVal)
		if err != nil {
			return err
		}
		if ok {
			w.Header().Set(h.name, v)
		}
	}

	if spec.noBody || statusMustNotHaveBody(status) {
		w.WriteHeader(status)
		return nil
	}
	return WriteSuccessWithCodecs(w, req, status, spec.bodyValue(outVal), codecs)
}

func WriteJSON(w http.ResponseWriter, status int, v any) error {
	return JSONCodec{}.encodeRaw(w, status, v)
}

// DataResponse is the standard envelope for all successful API responses.
type DataResponse struct {
	Data any `json:"data"`
}

// WriteSuccess writes a successful JSON response wrapped in the standard DataResponse envelope.
func WriteSuccess(w http.ResponseWriter, status int, data any) error {
	return defaultJSONCodec.EncodeSuccess(w, status, data)
}

type inputBinding struct {
	read      func(*http.Request) (string, bool, error)
	readSlice func(*http.Request) ([]string, bool, error)
	readFile  func(*http.Request) (*multipart.FileHeader, bool, error)
	readFiles func(*http.Request) ([]*multipart.FileHeader, bool, error)
	set       func(reflect.Value, string) error
	setSlice  func(reflect.Value, []string) error
	setFile   func(reflect.Value, *multipart.FileHeader) error
	setFiles  func(reflect.Value, []*multipart.FileHeader) error
	name      string
	location  string
	source    string
	index     int
	required  bool
}

type bodyBindingMode uint8

const (
	bodyModeNone bodyBindingMode = iota
	bodyModeStruct
	bodyModeField
	bodyModeMultipart
)

type RequiredJSONField struct {
	Pointer string
	Path    []string
}

type inputSpec struct {
	typ                reflect.Type
	bodyType           reflect.Type
	validationSpec     atomic.Pointer[validation.StructSpec]
	bindings           []inputBinding
	requiredBodyFields []RequiredJSONField
	bodyIndex          int
	bodyMode           bodyBindingMode
	bodyRequired       bool
}

const multipartMaxMemory = 32 << 20

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
			ft := validation.IndirectType(f.Type)
			if ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.String {
				spec.bindings = append(spec.bindings, inputBinding{
					index:    i,
					name:     name,
					location: "query parameter",
					source:   "query",
					required: validation.FieldRequired(f, "query"),
					readSlice: func(req *http.Request) ([]string, bool, error) {
						values, ok := req.URL.Query()[name]
						if !ok || len(values) == 0 {
							return nil, false, nil
						}
						return values, true, nil
					},
					setSlice: func(v reflect.Value, values []string) error {
						return setStringSliceValue(v, values)
					},
				})
			} else {
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
			}
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
		case f.Tag.Get("form") != "":
			name := validation.TagName(f.Tag.Get("form"))
			spec.bodyMode = bodyModeMultipart
			ft := validation.IndirectType(f.Type)
			if ft.Kind() == reflect.Slice {
				spec.bindings = append(spec.bindings, inputBinding{
					index:    i,
					name:     name,
					location: "form field",
					source:   "form",
					required: validation.FieldRequired(f, "form"),
					readSlice: func(req *http.Request) ([]string, bool, error) {
						if err := ensureMultipartForm(req); err != nil {
							return nil, false, err
						}
						if req.MultipartForm == nil {
							return nil, false, nil
						}
						values := req.MultipartForm.Value[name]
						if len(values) == 0 {
							return nil, false, nil
						}
						return values, true, nil
					},
					setSlice: setStringSliceValue,
				})
			} else {
				spec.bindings = append(spec.bindings, inputBinding{
					index:    i,
					name:     name,
					location: "form field",
					source:   "form",
					required: validation.FieldRequired(f, "form"),
					read: func(req *http.Request) (string, bool, error) {
						if err := ensureMultipartForm(req); err != nil {
							return "", false, err
						}
						if req.MultipartForm == nil {
							return "", false, nil
						}
						values := req.MultipartForm.Value[name]
						if len(values) == 0 {
							return "", false, nil
						}
						return values[0], true, nil
					},
					set: setStringValue,
				})
			}
		case f.Tag.Get("file") != "":
			name := validation.TagName(f.Tag.Get("file"))
			spec.bodyMode = bodyModeMultipart
			spec.bindings = append(spec.bindings, inputBinding{
				index:    i,
				name:     name,
				location: "file field",
				source:   "file",
				required: validation.FieldRequired(f, "file"),
				readFile: func(req *http.Request) (*multipart.FileHeader, bool, error) {
					files, ok, err := readMultipartFiles(req, name)
					if err != nil || !ok {
						return nil, false, err
					}
					return files[0], true, nil
				},
				setFile: setMultipartFileValue,
			})
		case f.Tag.Get("files") != "":
			name := validation.TagName(f.Tag.Get("files"))
			spec.bodyMode = bodyModeMultipart
			spec.bindings = append(spec.bindings, inputBinding{
				index:    i,
				name:     name,
				location: "file fields",
				source:   "files",
				required: validation.FieldRequired(f, "files"),
				readFiles: func(req *http.Request) ([]*multipart.FileHeader, bool, error) {
					return readMultipartFiles(req, name)
				},
				setFiles: setMultipartFilesValue,
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
	if spec.bodyMode == bodyModeMultipart {
		for _, binding := range spec.bindings {
			if binding.required && (binding.source == "form" || binding.source == "file" || binding.source == "files") {
				spec.bodyRequired = true
				break
			}
		}
	}

	return spec
}

func decodeRequestBody(req *http.Request, target reflect.Value, spec *inputSpec, codecs []Codec) error {
	if req.Body == nil {
		if spec.bodyRequired {
			validationErr := &validation.Error{}
			validationErr.AddRequired("")
			return validationErr
		}
		return nil
	}
	codec, err := codecForRequestContentType(codecs, req.Header.Get("Content-Type"))
	if err != nil {
		return err
	}
	if !isJSONCodec(codec) {
		return decodeRequestBodyWithCodec(req, target, spec, codec)
	}

	bodyBytes, err := readJSONBody(req)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return fmt.Errorf("request body too large on %s %s: limit is %d bytes", req.Method, req.URL.Path, maxBytesErr.Limit)
		}
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

func decodeRequestBodyWithCodec(req *http.Request, target reflect.Value, spec *inputSpec, codec Codec) error {
	switch spec.bodyMode {
	case bodyModeStruct:
		if err := codec.DecodeRequest(req, target.Addr().Interface()); err != nil {
			return fmt.Errorf("failed to decode body: %w", err)
		}
	case bodyModeField:
		field := target.Field(spec.bodyIndex)
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			if err := codec.DecodeRequest(req, field.Interface()); err != nil {
				return fmt.Errorf("failed to decode body: %w", err)
			}
		} else {
			if err := codec.DecodeRequest(req, field.Addr().Interface()); err != nil {
				return fmt.Errorf("failed to decode body: %w", err)
			}
		}
	}
	return nil
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

func setStringSliceValue(v reflect.Value, values []string) error {
	if v.Kind() == reflect.Pointer {
		if len(values) == 0 {
			return nil
		}
		v.Set(reflect.New(v.Type().Elem()))
		return setStringSliceValue(v.Elem(), values)
	}
	if v.Kind() != reflect.Slice {
		return fmt.Errorf("expected slice, got %s", v.Type())
	}
	for _, val := range values {
		elem := reflect.New(v.Type().Elem()).Elem()
		if err := setStringValue(elem, val); err != nil {
			return err
		}
		v.Set(reflect.Append(v, elem))
	}
	return nil
}

func setMultipartFileValue(v reflect.Value, file *multipart.FileHeader) error {
	if !v.CanSet() || v.Type() != reflect.TypeFor[*multipart.FileHeader]() {
		return fmt.Errorf("expected *multipart.FileHeader, got %s", v.Type())
	}
	v.Set(reflect.ValueOf(file))
	return nil
}

func setMultipartFilesValue(v reflect.Value, files []*multipart.FileHeader) error {
	if !v.CanSet() || v.Type() != reflect.TypeFor[[]*multipart.FileHeader]() {
		return fmt.Errorf("expected []*multipart.FileHeader, got %s", v.Type())
	}
	v.Set(reflect.ValueOf(files))
	return nil
}

func ensureMultipartForm(req *http.Request) error {
	return EnsureMultipartForm(req)
}

// EnsureMultipartForm parses req as multipart/form-data if it has not already
// been parsed. It is used by reflection and generated request binders.
func EnsureMultipartForm(req *http.Request) error {
	if req.MultipartForm != nil {
		return nil
	}
	if err := validateMultipartContentType(req.Header.Get("Content-Type")); err != nil {
		return err
	}
	if req.Body == nil {
		return nil
	}
	if err := req.ParseMultipartForm(multipartMaxMemory); err != nil {
		if errors.Is(err, http.ErrNotMultipart) {
			return fmt.Errorf("unsupported content type %q", req.Header.Get("Content-Type"))
		}
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return fmt.Errorf("request body too large on %s %s: limit is %d bytes", req.Method, req.URL.Path, maxBytesErr.Limit)
		}
		return fmt.Errorf("failed to parse multipart form: %w", err)
	}
	return nil
}

func readMultipartFiles(req *http.Request, name string) ([]*multipart.FileHeader, bool, error) {
	return ReadMultipartFiles(req, name)
}

// ReadMultipartFormValues reads all values for a multipart form field.
func ReadMultipartFormValues(req *http.Request, name string) ([]string, bool, error) {
	if err := EnsureMultipartForm(req); err != nil {
		return nil, false, err
	}
	if req.MultipartForm == nil {
		return nil, false, nil
	}
	values := req.MultipartForm.Value[name]
	if len(values) == 0 {
		return nil, false, nil
	}
	return values, true, nil
}

// ReadMultipartFiles reads all uploaded files for a multipart file field.
func ReadMultipartFiles(req *http.Request, name string) ([]*multipart.FileHeader, bool, error) {
	if err := EnsureMultipartForm(req); err != nil {
		return nil, false, err
	}
	if req.MultipartForm == nil {
		return nil, false, nil
	}
	files := req.MultipartForm.File[name]
	if len(files) == 0 {
		return nil, false, nil
	}
	return files, true, nil
}

func ReadRequestJSONBody(req *http.Request) ([]byte, error) {
	return JSONCodec{}.ReadRequest(req)
}

func ReadRequestJSONBodyFast(req *http.Request) ([]byte, error) {
	return JSONCodec{}.ReadRequest(req)
}

func (JSONCodec) ReadRequest(req *http.Request) ([]byte, error) {
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
			return nil, fmt.Errorf("request body too large on %s %s: limit is %d bytes", req.Method, req.URL.Path, maxBytesErr.Limit)
		}
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	if isBlankJSONBody(bodyBytes) {
		return nil, nil
	}
	return bodyBytes, nil
}

func DecodeRequestJSONBodyFast(req *http.Request, dst any) error {
	return defaultJSONCodec.DecodeRequest(req, dst)
}

func decodeRequestJSONBodyFast(req *http.Request, dst any) error {
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
			return fmt.Errorf("request body too large on %s %s: limit is %d bytes", req.Method, req.URL.Path, maxBytesErr.Limit)
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
	return JSONCodec{}.DecodeRequestStrict(req, dst, bodyRequired, required)
}

func (JSONCodec) DecodeRequestStrict(req *http.Request, dst any, bodyRequired bool, required []RequiredJSONField) error {
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
			return fmt.Errorf("request body too large on %s %s: limit is %d bytes", req.Method, req.URL.Path, maxBytesErr.Limit)
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
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return NewHTTPError(http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported content type %q", contentType))
	}
	mediaType = strings.ToLower(mediaType)
	if mediaType != "" && mediaType != "application/json" && !strings.HasSuffix(mediaType, "+json") {
		return NewHTTPError(http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported content type %q", mediaType))
	}
	return nil
}

func validateMultipartContentType(contentType string) error {
	if contentType == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return NewHTTPError(http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported content type %q", contentType))
	}
	mediaType = strings.ToLower(mediaType)
	if mediaType != "" && mediaType != "multipart/form-data" {
		return NewHTTPError(http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported content type %q", mediaType))
	}
	return nil
}

func ValidateJSONContentType(contentType string) error {
	return validateJSONContentType(contentType)
}

// NegotiateResponseContentType returns 406 when req's Accept header does not
// allow any of the response media types. An absent Accept header accepts any
// response. Passing no media types is treated as a response with no negotiated
// body.
func NegotiateResponseContentType(req *http.Request, mediaTypes ...string) error {
	if req == nil || len(mediaTypes) == 0 {
		return nil
	}
	for _, mediaType := range mediaTypes {
		if acceptsMediaType(req.Header.Get("Accept"), mediaType) {
			return nil
		}
	}
	return NewHTTPError(
		http.StatusNotAcceptable,
		fmt.Sprintf("not acceptable: client does not accept %q", strings.Join(mediaTypes, ", ")),
	)
}

func acceptsMediaType(accept, mediaType string) bool {
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return true
	}
	wantType, wantSubtype, ok := splitMediaType(mediaType)
	if !ok {
		return false
	}
	bestSpecificity := -1
	bestQ := 0.0
	for part := range strings.SplitSeq(accept, ",") {
		mediaRange, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		gotType, gotSubtype, ok := splitMediaType(mediaRange)
		if !ok {
			continue
		}
		specificity := mediaRangeSpecificity(gotType, gotSubtype, wantType, wantSubtype)
		if specificity < 0 {
			continue
		}
		q := 1.0
		if rawQ := params["q"]; rawQ != "" {
			parsed, err := strconv.ParseFloat(rawQ, 64)
			if err != nil || parsed < 0 {
				continue
			}
			if parsed > 1 {
				parsed = 1
			}
			q = parsed
		}
		if specificity > bestSpecificity {
			bestSpecificity = specificity
			bestQ = q
			continue
		}
		if specificity == bestSpecificity && q > bestQ {
			bestQ = q
		}
	}
	return bestSpecificity >= 0 && bestQ > 0
}

func splitMediaType(mediaType string) (string, string, bool) {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	typ, subtype, ok := strings.Cut(mediaType, "/")
	if !ok || typ == "" || subtype == "" {
		return "", "", false
	}
	return typ, subtype, true
}

func mediaRangeSpecificity(gotType, gotSubtype, wantType, wantSubtype string) int {
	switch {
	case gotType == "*" && gotSubtype == "*":
		return 0
	case gotType == wantType && gotSubtype == "*":
		return 1
	case gotType == wantType && strings.HasPrefix(gotSubtype, "*+") && strings.HasSuffix(wantSubtype, gotSubtype[1:]):
		return 2
	case gotType == wantType && gotSubtype == wantSubtype:
		return 3
	default:
		return -1
	}
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
	get      func(reflect.Value) (string, bool, error)
	name     string
	index    int
	required bool
}

type outputSpec struct {
	typ           reflect.Type
	bodyValue     func(reflect.Value) any
	headers       []outputHeaderField
	bodyIndex     int
	statusIndex   int
	defaultStatus int
	noBody        bool
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
	case bodyModeMultipart:
		bodySchema := &openapi.Schema{Type: "object", Properties: map[string]*openapi.Schema{}}
		seen := map[string]bool{}
		for _, binding := range spec.bindings {
			if binding.source != "form" && binding.source != "file" && binding.source != "files" {
				continue
			}
			if seen[binding.name] {
				continue
			}
			seen[binding.name] = true
			f := spec.typ.Field(binding.index)
			var fieldSchema *openapi.Schema
			switch binding.source {
			case "file":
				fieldSchema = &openapi.Schema{Type: "string", Format: "binary"}
			case "files":
				fieldSchema = &openapi.Schema{
					Type:  "array",
					Items: &openapi.Schema{Type: "string", Format: "binary"},
				}
			default:
				fieldSchema = openapi.CloneSchema(registry.Schema(f.Type))
			}
			openapi.ApplyFieldSchemaMetadata(fieldSchema, f)
			if binding.source == "file" {
				fieldSchema.Type = "string"
				fieldSchema.Format = "binary"
			}
			if binding.source == "files" && fieldSchema.Items != nil {
				fieldSchema.Items.Type = "string"
				fieldSchema.Items.Format = "binary"
			}
			bodySchema.Properties[binding.name] = fieldSchema
			if binding.required {
				bodySchema.Required = append(bodySchema.Required, binding.name)
			}
		}
		if len(bodySchema.Required) == 0 {
			bodySchema.Required = nil
		}
		return bodySchema
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
	if path == "" || !strings.ContainsAny(path, ":*") {
		return path
	}
	return rewriteSegments(
		path,
		func(name string) string { return "{" + name + "}" },
		func(name string) string {
			if name == "" {
				name = wildcardParamName
			}
			return "{" + name + "}"
		},
	)
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
