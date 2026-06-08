package plugins

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMeteredWriter_Ignores1xx verifies that informational 1xx responses are
// not latched as the final status and do not mark the response written — which
// would otherwise corrupt metrics and suppress a later error under InstrumentHTTP.
func TestMeteredWriter_Ignores1xx(t *testing.T) {
	mw := &meteredWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}

	mw.WriteHeader(http.StatusEarlyHints) // 103
	if mw.Written() {
		t.Error("1xx must not mark the response as written")
	}
	if mw.status != http.StatusOK {
		t.Errorf("1xx latched as status: got %d", mw.status)
	}

	mw.WriteHeader(http.StatusCreated) // 201 final
	if !mw.Written() || mw.status != http.StatusCreated {
		t.Errorf("final status not captured: status=%d written=%v", mw.status, mw.Written())
	}
}
