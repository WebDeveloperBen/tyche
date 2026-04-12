package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"sync"
)

type GeneratedRouteCodec struct {
	Parse func(*http.Request) (any, error)
	Write func(http.ResponseWriter, *http.Request, any) error
}

type generatedCodecEntry struct {
	meta  GeneratedRouteMeta
	codec GeneratedRouteCodec
}

var generatedCodecRegistry struct {
	mu     sync.RWMutex
	codecs []generatedCodecEntry
}

var encoderBufPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
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

	inputKey := generatedTypeKey(inputType)
	outputKey := generatedTypeKey(outputType)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	buf := encoderBufPool.Get().(*bytes.Buffer)
	buf.Reset()

	defer encoderBufPool.Put(buf)

	enc := json.NewEncoder(buf)
	if err := enc.Encode(out); err != nil {
		return err
	}

	_, writeErr := w.Write(buf.Bytes())
	return writeErr
}

func AcquireGeneratedJSONBuffer() *[]byte {
	buf := generatedJSONBytePool.Get().(*[]byte)
	*buf = (*buf)[:0]
	return buf
}

func ReleaseGeneratedJSONBuffer(buf *[]byte) {
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
