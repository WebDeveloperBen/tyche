// Package servertest provides helpers for testing tyche routers and handlers
// with the standard library's httptest, removing the boilerplate of building
// requests and unwrapping the standard DataResponse envelope.
//
//	client := servertest.New(t, router)
//	resp := client.POST("/v1/users", User{Name: "Ada"})
//	resp.AssertStatus(http.StatusCreated)
//	got := servertest.DecodeData[User](t, resp)
package servertest

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

// Client issues requests against an http.Handler (typically a *server.Router)
// using httptest, recording responses for assertions.
type Client struct {
	tb      testing.TB
	handler http.Handler
	// Header holds default headers applied to every request. Per-request
	// headers set via WithHeader take precedence.
	Header http.Header
}

// New returns a Client that drives handler. tb is used to fail tests on
// request-construction or decoding errors.
func New(tb testing.TB, handler http.Handler) *Client {
	tb.Helper()
	if handler == nil {
		tb.Fatal("servertest: handler cannot be nil")
	}
	return &Client{tb: tb, handler: handler, Header: make(http.Header)}
}

// Response wraps a recorded response with assertion and decoding helpers.
type Response struct {
	*httptest.ResponseRecorder
	tb     testing.TB
	method string
	target string
}

// Do builds a request for the given method and target, marshaling body per the
// rules of [encodeBody], serves it, and returns the recorded Response.
func (c *Client) Do(method, target string, body any, opts ...RequestOption) *Response {
	c.tb.Helper()

	reader, contentType, err := encodeBody(body)
	if err != nil {
		c.tb.Fatalf("servertest: encode body for %s %s: %v", method, target, err)
	}

	req := httptest.NewRequest(method, target, reader)
	for key, values := range c.Header {
		for _, v := range values {
			req.Header.Add(key, v)
		}
	}
	if contentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, opt := range opts {
		opt(req)
	}

	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	return &Response{ResponseRecorder: rec, tb: c.tb, method: method, target: target}
}

// GET issues a GET request.
func (c *Client) GET(target string, opts ...RequestOption) *Response {
	c.tb.Helper()
	return c.Do(http.MethodGet, target, nil, opts...)
}

// POST issues a POST request with the given body.
func (c *Client) POST(target string, body any, opts ...RequestOption) *Response {
	c.tb.Helper()
	return c.Do(http.MethodPost, target, body, opts...)
}

// PUT issues a PUT request with the given body.
func (c *Client) PUT(target string, body any, opts ...RequestOption) *Response {
	c.tb.Helper()
	return c.Do(http.MethodPut, target, body, opts...)
}

// PATCH issues a PATCH request with the given body.
func (c *Client) PATCH(target string, body any, opts ...RequestOption) *Response {
	c.tb.Helper()
	return c.Do(http.MethodPatch, target, body, opts...)
}

// DELETE issues a DELETE request.
func (c *Client) DELETE(target string, opts ...RequestOption) *Response {
	c.tb.Helper()
	return c.Do(http.MethodDelete, target, nil, opts...)
}

// RequestOption mutates a request before it is served, e.g. to set headers.
type RequestOption func(*http.Request)

// WithHeader sets a request header.
func WithHeader(key, value string) RequestOption {
	return func(r *http.Request) { r.Header.Set(key, value) }
}

// WithBearerToken sets an Authorization: Bearer header.
func WithBearerToken(token string) RequestOption {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

// AssertStatus fails the test if the response status does not match want. It
// returns the Response for chaining.
func (r *Response) AssertStatus(want int) *Response {
	r.tb.Helper()
	if r.Code != want {
		r.tb.Fatalf("servertest: %s %s: expected status %d, got %d (body: %s)",
			r.method, r.target, want, r.Code, r.bodyExcerpt())
	}
	return r
}

func (r *Response) bodyExcerpt() string {
	const limit = 512
	body := r.Body.String()
	if len(body) <= limit {
		return body
	}
	// Trim back to a rune boundary so the excerpt stays valid UTF-8.
	truncated := body[:limit]
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + "…"
}

// dataEnvelope mirrors server.DataResponse for generic decoding. It is kept
// local so the helper stays decoupled from the server package's concrete type.
type dataEnvelope[T any] struct {
	Data T `json:"data"`
}

// DecodeData decodes the response body as the standard {"data": T} envelope and
// returns the unwrapped value. It fails the test on a decoding error.
func DecodeData[T any](tb testing.TB, r *Response) T {
	tb.Helper()
	var env dataEnvelope[T]
	if err := json.Unmarshal(r.Body.Bytes(), &env); err != nil {
		tb.Fatalf("servertest: decode data envelope (%s %s): %v (body: %s)",
			r.method, r.target, err, r.bodyExcerpt())
	}
	return env.Data
}

// Decode decodes the raw response body into dst (no envelope unwrapping). It
// fails the test on a decoding error.
func Decode[T any](tb testing.TB, r *Response) T {
	tb.Helper()
	var v T
	if err := json.Unmarshal(r.Body.Bytes(), &v); err != nil {
		tb.Fatalf("servertest: decode body (%s %s): %v (body: %s)",
			r.method, r.target, err, r.bodyExcerpt())
	}
	return v
}

// Problem is the RFC 9457 problem+json shape that tyche emits for errors.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
	// Errors carries validation problem details when present.
	Errors []ValidationProblem `json:"errors"`
}

// ValidationProblem is a single field-level validation error.
type ValidationProblem struct {
	Pointer string `json:"pointer"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DecodeProblem decodes the response body as a problem+json document. It fails
// the test on a decoding error.
func DecodeProblem(tb testing.TB, r *Response) Problem {
	tb.Helper()
	var p Problem
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
		tb.Fatalf("servertest: decode problem (%s %s): %v (body: %s)",
			r.method, r.target, err, r.bodyExcerpt())
	}
	return p
}

// encodeBody turns a request body argument into a reader and content type.
// nil yields no body; string, []byte, and io.Reader are sent verbatim with no
// content type; anything else is marshaled to JSON.
func encodeBody(body any) (io.Reader, string, error) {
	switch b := body.(type) {
	case nil:
		return nil, "", nil
	case io.Reader:
		return b, "", nil
	case string:
		return strings.NewReader(b), "", nil
	case []byte:
		return bytes.NewReader(b), "", nil
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return nil, "", err
		}
		return bytes.NewReader(data), "application/json", nil
	}
}
