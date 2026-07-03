package server

import (
	"net/http"
	"reflect"
	"sync"
)

type GeneratedRouteCodec struct {
	Parse           func(*http.Request) (any, error)
	Write           func(http.ResponseWriter, *http.Request, any) error
	ParseWithCodecs func(*http.Request, []Codec) (any, error)
	WriteWithCodecs func(http.ResponseWriter, *http.Request, any, []Codec) error
}

type generatedCodecEntry struct {
	codec GeneratedRouteCodec
	meta  GeneratedRouteMeta
}

var generatedCodecRegistry struct {
	codecs []generatedCodecEntry
	mu     sync.RWMutex
}

var generatedJSONBytePool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 256)
		return &buf
	},
}

func RegisterGeneratedCodec(meta GeneratedRouteMeta, codec GeneratedRouteCodec) {
	generatedCodecRegistry.mu.Lock()
	defer generatedCodecRegistry.mu.Unlock()

	identity := generatedRouteIdentity(meta)
	for _, entry := range generatedCodecRegistry.codecs {
		if generatedRouteIdentity(entry.meta) == identity {
			panic("generated codec already registered for route: " + identity)
		}
	}
	generatedCodecRegistry.codecs = append(generatedCodecRegistry.codecs, generatedCodecEntry{meta: meta, codec: codec})
}

func generatedCodec(op Operation, inputType, outputType reflect.Type) (GeneratedRouteCodec, bool) {
	generatedCodecRegistry.mu.RLock()
	defer generatedCodecRegistry.mu.RUnlock()

	inputKey := GeneratedTypeKey(inputType)
	outputKey := GeneratedTypeKey(outputType)
	for _, entry := range generatedCodecRegistry.codecs {
		if entry.meta.OperationID == op.OperationID &&
			entry.meta.Method == op.Method &&
			entry.meta.Path == op.Path &&
			(entry.meta.InputTypeKey == "" || entry.meta.InputTypeKey == inputKey) &&
			(entry.meta.OutputTypeKey == "" || entry.meta.OutputTypeKey == outputKey) {
			return entry.codec, true
		}
	}
	return GeneratedRouteCodec{}, false
}

func WriteTypedResponse[O any](w http.ResponseWriter, out *O) error {
	if out == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return defaultJSONCodec.EncodeSuccess(w, http.StatusOK, out)
}

func AcquireGeneratedJSONBuffer() *[]byte {
	return JSONCodec{}.AcquireGeneratedSuccessBuffer()
}

func ReleaseGeneratedJSONBuffer(buf *[]byte) {
	JSONCodec{}.ReleaseGeneratedSuccessBuffer(buf)
}
