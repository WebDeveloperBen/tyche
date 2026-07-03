package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// Codec decodes request bodies and writes successful typed responses for one
// media type. JSONCodec is registered implicitly as the default codec.
type Codec interface {
	MediaType() string
	DecodeRequest(*http.Request, any) error
	EncodeSuccess(http.ResponseWriter, int, any) error
}

// JSONCodec is the default application/json codec used by typed routes.
type JSONCodec struct{}

func (JSONCodec) MediaType() string { return "application/json" }

func (JSONCodec) DecodeRequest(req *http.Request, dst any) error {
	return decodeRequestJSONBodyFast(req, dst)
}

func (JSONCodec) EncodeSuccess(w http.ResponseWriter, status int, data any) error {
	return WriteJSON(w, status, DataResponse{Data: data})
}

// AcquireGeneratedSuccessBuffer returns a scratch buffer for generated JSON
// success response writers. Release it with ReleaseGeneratedSuccessBuffer.
func (JSONCodec) AcquireGeneratedSuccessBuffer() *[]byte {
	buf := generatedJSONBytePool.Get().(*[]byte)
	*buf = (*buf)[:0]
	return buf
}

func (JSONCodec) ReleaseGeneratedSuccessBuffer(buf *[]byte) {
	if buf == nil {
		return
	}
	if cap(*buf) > 4096 {
		*buf = make([]byte, 0, 256)
	} else {
		*buf = (*buf)[:0]
	}
	generatedJSONBytePool.Put(buf)
}

func (JSONCodec) AppendString(dst []byte, v string) []byte {
	return strconv.AppendQuote(dst, v)
}

func (JSONCodec) AppendBool(dst []byte, v bool) []byte {
	return strconv.AppendBool(dst, v)
}

func (JSONCodec) AppendInt(dst []byte, v int64) []byte {
	return strconv.AppendInt(dst, v, 10)
}

func (JSONCodec) AppendUint(dst []byte, v uint64) []byte {
	return strconv.AppendUint(dst, v, 10)
}

func (JSONCodec) AppendFloat(dst []byte, v float64) []byte {
	return strconv.AppendFloat(dst, v, 'f', -1, 64)
}

func (JSONCodec) WriteGeneratedSuccess(w http.ResponseWriter, status int, body []byte) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err := w.Write(body)
	return err
}

func (JSONCodec) encodeRaw(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(v)
}

var defaultJSONCodec Codec = JSONCodec{}

func codecMediaType(codec Codec) string {
	if codec == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(codec.MediaType()))
}

func codecMediaTypes(codecs []Codec) []string {
	mediaTypes := make([]string, 0, len(codecs))
	for _, codec := range codecs {
		mediaType := codecMediaType(codec)
		if mediaType == "" {
			continue
		}
		mediaTypes = append(mediaTypes, mediaType)
	}
	if len(mediaTypes) == 0 {
		return []string{"application/json"}
	}
	return mediaTypes
}

func codecForRequestContentType(codecs []Codec, contentType string) (Codec, error) {
	if strings.TrimSpace(contentType) == "" {
		return jsonCodecFrom(codecs), nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, NewHTTPError(http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported content type %q", contentType))
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	for _, codec := range codecs {
		if codecMediaType(codec) == mediaType {
			return codec, nil
		}
	}
	if mediaType == "application/json" || strings.HasSuffix(mediaType, "+json") {
		return jsonCodecFrom(codecs), nil
	}
	return nil, NewHTTPError(http.StatusUnsupportedMediaType, fmt.Sprintf("unsupported content type %q", mediaType))
}

func codecForAccept(codecs []Codec, accept string) (Codec, bool) {
	for _, codec := range codecs {
		mediaType := codecMediaType(codec)
		if mediaType == "" {
			continue
		}
		if acceptsMediaType(accept, mediaType) {
			return codec, true
		}
	}
	return nil, false
}

func responseCodecForRequest(req *http.Request, codecs []Codec) (Codec, error) {
	accept := ""
	if req != nil {
		accept = req.Header.Get("Accept")
	}
	codec, ok := codecForAccept(codecs, accept)
	if ok {
		return codec, nil
	}
	mediaTypes := codecMediaTypes(codecs)
	return nil, NewHTTPError(
		http.StatusNotAcceptable,
		fmt.Sprintf("not acceptable: client does not accept %q", strings.Join(mediaTypes, ", ")),
	)
}

func UseJSONCodecForRequest(req *http.Request, codecs []Codec) (bool, error) {
	if req == nil || req.Body == nil {
		return true, nil
	}
	codec, err := codecForRequestContentType(codecs, req.Header.Get("Content-Type"))
	if err != nil {
		return false, err
	}
	return isJSONCodec(codec), nil
}

func UseJSONCodecForResponse(req *http.Request, codecs []Codec) (bool, error) {
	codec, err := responseCodecForRequest(req, codecs)
	if err != nil {
		return false, err
	}
	return isJSONCodec(codec), nil
}

func WriteSuccessWithCodecs(w http.ResponseWriter, req *http.Request, status int, data any, codecs []Codec) error {
	codec, err := responseCodecForRequest(req, codecs)
	if err != nil {
		return err
	}
	if req != nil && req.Method == http.MethodHead {
		w.Header().Set("Content-Type", codecMediaType(codec))
		w.WriteHeader(status)
		return nil
	}
	return codec.EncodeSuccess(w, status, data)
}

func jsonCodecFrom(codecs []Codec) Codec {
	for _, codec := range codecs {
		if isJSONCodec(codec) {
			return codec
		}
	}
	return JSONCodec{}
}

func isJSONCodec(codec Codec) bool {
	switch codec.(type) {
	case JSONCodec, *JSONCodec:
		return true
	default:
		return false
	}
}

func hasNonJSONCodec(codecs []Codec) bool {
	for _, codec := range codecs {
		if codecMediaType(codec) == "" {
			continue
		}
		if !isJSONCodec(codec) {
			return true
		}
	}
	return false
}

func codecsForMediaTypes(codecs []Codec, mediaTypes []string) ([]Codec, error) {
	if len(mediaTypes) == 0 {
		return codecs, nil
	}
	registered := make(map[string]Codec, len(codecs))
	for _, codec := range codecs {
		mediaType := codecMediaType(codec)
		if mediaType == "" {
			continue
		}
		registered[mediaType] = codec
	}

	out := make([]Codec, 0, len(mediaTypes))
	seen := make(map[string]struct{}, len(mediaTypes))
	for _, raw := range mediaTypes {
		mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid route content type %q: %w", raw, err)
		}
		mediaType = strings.ToLower(strings.TrimSpace(mediaType))
		if mediaType == "" {
			return nil, fmt.Errorf("invalid route content type %q", raw)
		}
		if _, ok := seen[mediaType]; ok {
			continue
		}
		codec, ok := registered[mediaType]
		if !ok {
			return nil, fmt.Errorf("route content type %q is not registered as a server codec", mediaType)
		}
		seen[mediaType] = struct{}{}
		out = append(out, codec)
	}
	if len(out) == 0 {
		return nil, errors.New("route content types did not include any registered codec")
	}
	return out, nil
}

func normalizeCodecs(codecs []Codec) []Codec {
	out := make([]Codec, 0, len(codecs)+1)
	seen := map[string]struct{}{}
	for _, codec := range codecs {
		if codec == nil {
			continue
		}
		mediaType := strings.ToLower(strings.TrimSpace(codec.MediaType()))
		if mediaType == "" {
			continue
		}
		if _, ok := seen[mediaType]; ok {
			continue
		}
		seen[mediaType] = struct{}{}
		out = append(out, codec)
	}
	if len(out) == 0 {
		return []Codec{JSONCodec{}}
	}
	return out
}

// Codecs returns the server-wide codecs configured on the API. JSONCodec is
// present by default.
func (a *API) Codecs() []Codec {
	if a == nil {
		return nil
	}
	return append([]Codec(nil), a.codecs...)
}
