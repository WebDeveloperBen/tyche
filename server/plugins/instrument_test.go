package plugins_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestInstrument_CapturesRequestInfo(t *testing.T) {
	var got plugins.RequestInfo
	router := server.NewRouter()
	router.Use(plugins.Instrument(plugins.ObserverFunc(func(i plugins.RequestInfo) {
		got = i
	})))

	router.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
		return nil
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/7", nil))

	if got.Method != http.MethodGet {
		t.Errorf("Method = %q", got.Method)
	}
	if got.Route != "/users/:id" {
		t.Errorf("Route = %q, want /users/:id", got.Route)
	}
	if got.Path != "/users/7" {
		t.Errorf("Path = %q", got.Path)
	}
	if got.Status != http.StatusCreated {
		t.Errorf("Status = %d, want 201", got.Status)
	}
	if got.Bytes != int64(len("hello")) {
		t.Errorf("Bytes = %d, want 5", got.Bytes)
	}
	if got.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", got.Duration)
	}
}

func TestInstrument_CapturesError(t *testing.T) {
	var got plugins.RequestInfo
	router := server.NewRouter()
	router.Use(plugins.Instrument(plugins.ObserverFunc(func(i plugins.RequestInfo) { got = i })))

	router.GET("/boom", func(w http.ResponseWriter, r *http.Request) error {
		return server.NewHTTPError(http.StatusBadGateway, "upstream down")
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	if got.Err == nil {
		t.Fatal("expected error to be captured")
	}
}

func TestInstrument_StreamingSafeFlusher(t *testing.T) {
	router := server.NewRouter()
	router.Use(plugins.Instrument(plugins.ObserverFunc(func(plugins.RequestInfo) {})))

	router.GET("/stream", func(w http.ResponseWriter, r *http.Request) error {
		stream, err := server.NewEventStream(w, r)
		if err != nil {
			return err
		}
		return stream.SendData(map[string]string{"ok": "yes"})
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))

	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("streaming broke through instrument wrapper: %q", rec.Header().Get("Content-Type"))
	}
	if got := rec.Body.String(); !strings.Contains(got, `"ok":"yes"`) {
		t.Errorf("stream body = %q", got)
	}
}

func TestInstrumentHTTP_AccurateStatusOnError(t *testing.T) {
	var got plugins.RequestInfo
	router := server.NewRouter()
	router.UseHTTP(plugins.InstrumentHTTP(plugins.ObserverFunc(func(i plugins.RequestInfo) { got = i })))

	router.GET("/boom", func(w http.ResponseWriter, r *http.Request) error {
		return server.NewHTTPError(http.StatusBadGateway, "upstream down")
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	// The whole point of InstrumentHTTP: it observes the FINAL status the
	// router rendered for the returned error, not the handler's default 200.
	if got.Status != http.StatusBadGateway {
		t.Errorf("InstrumentHTTP Status = %d, want 502 (final rendered status)", got.Status)
	}
	if got.Route != "/boom" {
		t.Errorf("Route = %q, want /boom", got.Route)
	}
	if got.Bytes == 0 {
		t.Error("expected the error response body bytes to be counted")
	}
}

func TestInstrumentHTTP_CapturesNotFound(t *testing.T) {
	var got plugins.RequestInfo
	router := server.NewRouter()
	router.UseHTTP(plugins.InstrumentHTTP(plugins.ObserverFunc(func(i plugins.RequestInfo) { got = i })))

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/missing", nil))

	if got.Status != http.StatusNotFound {
		t.Errorf("Status = %d, want 404", got.Status)
	}
}

func TestInstrumentHTTP_StreamingSafe(t *testing.T) {
	router := server.NewRouter()
	router.UseHTTP(plugins.InstrumentHTTP(plugins.ObserverFunc(func(plugins.RequestInfo) {})))
	router.GET("/stream", func(w http.ResponseWriter, r *http.Request) error {
		stream, err := server.NewEventStream(w, r)
		if err != nil {
			return err
		}
		return stream.SendData(map[string]string{"ok": "yes"})
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("streaming broke through InstrumentHTTP wrapper: %q", rec.Header().Get("Content-Type"))
	}
}

func TestInstrument_NilObserverIsNoop(t *testing.T) {
	router := server.NewRouter()
	router.Use(plugins.Instrument(nil))
	router.GET("/x", func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
}
