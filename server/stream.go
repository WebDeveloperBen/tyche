package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/webdeveloperben/tyche/server/openapi"
	"github.com/webdeveloperben/tyche/server/validation"
)

// ErrStreamClosed is returned by [EventStream] methods after the stream has
// been closed.
var ErrStreamClosed = errors.New("server: event stream is closed")

// SSEEvent is a single Server-Sent Event frame.
//
// Data is encoded as follows: a string or []byte is written verbatim; nil emits
// no data field; any other value is marshaled to JSON. Multi-line data is
// split across multiple "data:" lines per the SSE specification.
type SSEEvent struct {
	// ID sets the event's "id" field, used by clients as the
	// Last-Event-ID on reconnect. Optional.
	ID string
	// Event sets the event's "event" field (the event type). Optional;
	// defaults to "message" on the client when empty.
	Event string
	// Data is the event payload. See [SSEEvent] for encoding rules.
	Data any
	// Retry, when > 0, sets the client's reconnection time in milliseconds.
	Retry int
}

var sseBufPool = sync.Pool{
	New: func() any { return &bytes.Buffer{} },
}

// EventStream writes a Server-Sent Events (text/event-stream) response. Obtain
// one with [NewEventStream] inside a regular [HandlerFunc], or use
// [RegisterStream] for a typed, OpenAPI-documented streaming endpoint.
//
// An EventStream is not safe for concurrent use by multiple goroutines; send
// from a single goroutine, or guard sends with your own synchronization.
type EventStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
	ctx     context.Context
	mu      sync.Mutex
	closed  bool
}

// NewEventStream upgrades the response to a Server-Sent Events stream. It sets
// the appropriate headers (Content-Type: text/event-stream, no caching, and a
// hint to disable proxy buffering), writes a 200 status, and flushes so the
// client sees the response head immediately.
//
// It returns an error only if the response writer does not support flushing, in
// which case no bytes are written and the caller may still produce a normal
// error response.
func NewEventStream(w http.ResponseWriter, r *http.Request) (*EventStream, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("server: response writer does not support streaming (http.Flusher)")
	}

	h := w.Header()
	if h.Get("Content-Type") == "" {
		h.Set("Content-Type", "text/event-stream")
	}
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Disable response buffering in common reverse proxies (nginx) so events
	// are delivered as they are flushed rather than batched.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	return &EventStream{w: w, flusher: flusher, ctx: ctx}, nil
}

// Context returns the request context associated with the stream. Callers
// should stop sending when it is done (the client disconnected or a deadline
// elapsed).
func (s *EventStream) Context() context.Context {
	if s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

// Send writes a single event frame and flushes it to the client. A write error
// (typically a disconnected client) marks the stream closed and is returned so
// the caller can stop.
func (s *EventStream) Send(event SSEEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStreamClosed
	}

	buf := sseBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= 64*1024 {
			sseBufPool.Put(buf)
		}
	}()

	if event.ID != "" {
		writeSSEField(buf, "id", event.ID)
	}
	if event.Event != "" {
		writeSSEField(buf, "event", event.Event)
	}
	if event.Retry > 0 {
		buf.WriteString("retry: ")
		buf.WriteString(strconv.Itoa(event.Retry))
		buf.WriteByte('\n')
	}

	if event.Data != nil {
		data, err := encodeSSEData(event.Data)
		if err != nil {
			return err
		}
		// Each line of the payload becomes its own "data:" field so embedded
		// newlines survive the SSE framing.
		for line := range strings.SplitSeq(string(data), "\n") {
			writeSSEField(buf, "data", line)
		}
	}

	buf.WriteByte('\n')

	if _, err := s.w.Write(buf.Bytes()); err != nil {
		s.closed = true
		return err
	}
	s.flusher.Flush()
	return nil
}

// SendData sends an event whose only field is the JSON-encoded payload. It is
// shorthand for Send(SSEEvent{Data: v}).
func (s *EventStream) SendData(v any) error {
	return s.Send(SSEEvent{Data: v})
}

// Comment writes an SSE comment line (": text"). Comments are ignored by
// clients and are commonly used as keep-alive pings to keep idle connections
// open through proxies.
func (s *EventStream) Comment(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrStreamClosed
	}
	buf := sseBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() <= 64*1024 {
			sseBufPool.Put(buf)
		}
	}()
	for line := range strings.SplitSeq(text, "\n") {
		buf.WriteString(": ")
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
	if _, err := s.w.Write(buf.Bytes()); err != nil {
		s.closed = true
		return err
	}
	s.flusher.Flush()
	return nil
}

// Flush forces any buffered data out to the client. Send and Comment already
// flush; Flush is exposed for callers writing through other means.
func (s *EventStream) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.flusher.Flush()
	}
}

func writeSSEField(buf *bytes.Buffer, field, value string) {
	buf.WriteString(field)
	buf.WriteString(": ")
	buf.WriteString(value)
	buf.WriteByte('\n')
}

func encodeSSEData(v any) ([]byte, error) {
	switch data := v.(type) {
	case string:
		return []byte(data), nil
	case []byte:
		return data, nil
	default:
		return json.Marshal(v)
	}
}

// Stream is a type-safe view over an [EventStream] whose data frames are values
// of type O. It is the stream type passed to a [StreamHandler]; for raw access
// (custom event IDs, retry hints, comments) use [Stream.Raw].
type Stream[O any] struct {
	es *EventStream
}

// Send marshals data to JSON and emits it as a single SSE data frame.
func (s *Stream[O]) Send(data O) error {
	return s.es.SendData(data)
}

// SendEvent emits data as a named event (the SSE "event" field).
func (s *Stream[O]) SendEvent(event string, data O) error {
	return s.es.Send(SSEEvent{Event: event, Data: data})
}

// Comment writes an SSE comment line, commonly used as a keep-alive ping.
func (s *Stream[O]) Comment(text string) error {
	return s.es.Comment(text)
}

// Context returns the request context; stop sending when it is done.
func (s *Stream[O]) Context() context.Context {
	return s.es.Context()
}

// Raw returns the underlying [EventStream] for full control over framing.
func (s *Stream[O]) Raw() *EventStream {
	return s.es
}

// StreamHandler handles a typed streaming request. The input I is parsed and
// validated from the request like any typed handler; the handler then writes
// events of type O to the provided [Stream]. O also documents the JSON shape of
// each event's data frame in the generated OpenAPI specification.
type StreamHandler[I, O any] func(ctx context.Context, in *I, stream *Stream[O]) error

// RegisterStream registers a typed Server-Sent Events endpoint. It panics on
// invalid setup; use [RegisterStreamE] for the error-returning form.
func RegisterStream[I, O any](grp RouteTarget, op Operation, handler StreamHandler[I, O], opts ...RouteOption) {
	if err := RegisterStreamE(grp, op, handler, opts...); err != nil {
		panic(err)
	}
}

// RegisterStreamE registers a typed Server-Sent Events endpoint. Unlike
// [RegisterE] (which requires a generated codec), streaming endpoints bind their
// input via reflection ([ParseRequest]) — there is no generated codec for a
// streamed response — and therefore always validate the input at runtime.
// Operation.SkipValidateRequest only suppresses the registration-time
// validation-rule check, not runtime input validation. The response is
// documented in OpenAPI as a text/event-stream whose data frames match type O.
//
// Root-, group-, and route-level middleware (via opts) all apply.
func RegisterStreamE[I, O any](grp RouteTarget, op Operation, handler StreamHandler[I, O], opts ...RouteOption) error {
	if grp == nil {
		return errors.New("group cannot be nil")
	}
	if handler == nil {
		return errors.New("stream handler cannot be nil")
	}

	inputType := reflect.TypeFor[I]()
	outputType := reflect.TypeFor[O]()
	if !op.SkipValidateRequest {
		if _, err := validation.Struct(inputType); err != nil {
			return fmt.Errorf("invalid input validation for %s: %w", validation.IndirectType(inputType), err)
		}
	}

	if op.Method == "" {
		op.Method = http.MethodGet
	}
	if op.Path == "" {
		return errors.New("operation path is required")
	}
	if op.OperationID == "" {
		op.OperationID = fmt.Sprintf("%s-%s", strings.ToLower(op.Method), sanitizeOperationID(op.Path))
	}
	for _, existing := range grp.apiOperations() {
		if existing.OperationID == op.OperationID {
			return fmt.Errorf("duplicate operation ID: %s", op.OperationID)
		}
	}

	httpHandler := func(w http.ResponseWriter, req *http.Request) error {
		input, err := ParseRequest[I](req)
		if err != nil {
			var validationErr *validation.Error
			if errors.As(err, &validationErr) {
				return validationErr
			}
			return NewHTTPError(http.StatusBadRequest, err.Error())
		}

		es, err := NewEventStream(w, req)
		if err != nil {
			return NewHTTPError(http.StatusInternalServerError, "streaming is not supported by this server")
		}

		return handler(req.Context(), input, &Stream[O]{es: es})
	}

	if err := grp.handleRoute(op.Method, op.Path, httpHandler, resolveRouteOptions(opts)); err != nil {
		return err
	}
	registerStreamOpenAPIOperation(grp, op, inputType, outputType)
	return nil
}

func registerStreamOpenAPIOperation(grp RouteTarget, op Operation, inputType, outputType reflect.Type) {
	doc := grp.apiDoc()
	registry := grp.apiSchemaRegistry()

	inputSchema := registry.Schema(inputType)
	outputSchema := registry.Schema(outputType)
	openAPIPath := ServerPathToOpenAPIPath(joinPath(grp.groupPrefix(), op.Path))

	responses := map[string]*openapi.Response{
		"200": {
			Description: "Server-sent events stream",
			Content: map[string]*openapi.MediaType{
				"text/event-stream": {Schema: openapi.CloneSchema(outputSchema)},
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

	docOp := &openapi.Operation{
		Summary:     op.Summary,
		Description: op.Description,
		OperationID: op.OperationID,
		Tags:        op.Tags,
		Parameters:  extractParameters(inputType, registry),
		RequestBody: extractRequestBody(inputType, registry),
		Responses:   responses,
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
		outputName = fmt.Sprintf("InlineEvent%s", sanitizeOperationID(openAPIPath))
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
