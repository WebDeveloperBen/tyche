package servertest_test

import (
	"net/http"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/servertest"
)

type user struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func newRouter() *server.Router {
	router := server.NewRouter()

	router.POST("/users", func(w http.ResponseWriter, r *http.Request) error {
		var in user
		if err := server.DecodeRequestJSONBodyFast(r, &in); err != nil {
			return server.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		in.ID = "u1"
		return server.WriteSuccess(w, http.StatusCreated, in)
	})

	router.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		return server.WriteSuccess(w, http.StatusOK, user{ID: server.Param(r, "id"), Name: "Ada"})
	})

	router.GET("/secret", func(w http.ResponseWriter, r *http.Request) error {
		if r.Header.Get("Authorization") != "Bearer t0ken" {
			return server.NewHTTPError(http.StatusUnauthorized, "missing token")
		}
		return server.WriteSuccess(w, http.StatusOK, user{ID: "me"})
	})

	return router
}

func TestClient_PostAndDecodeData(t *testing.T) {
	client := servertest.New(t, newRouter())

	resp := client.POST("/users", user{Name: "Ada"}).AssertStatus(http.StatusCreated)
	got := servertest.DecodeData[user](t, resp)

	if got.ID != "u1" || got.Name != "Ada" {
		t.Errorf("unexpected user: %+v", got)
	}
}

func TestClient_GetWithPathParam(t *testing.T) {
	client := servertest.New(t, newRouter())
	resp := client.GET("/users/42").AssertStatus(http.StatusOK)
	got := servertest.DecodeData[user](t, resp)
	if got.ID != "42" {
		t.Errorf("expected id 42, got %q", got.ID)
	}
}

func TestClient_DecodeProblem(t *testing.T) {
	client := servertest.New(t, newRouter())
	resp := client.GET("/secret").AssertStatus(http.StatusUnauthorized)
	problem := servertest.DecodeProblem(t, resp)
	if problem.Status != http.StatusUnauthorized {
		t.Errorf("problem.Status = %d", problem.Status)
	}
	if problem.Detail != "missing token" {
		t.Errorf("problem.Detail = %q", problem.Detail)
	}
}

func TestClient_RequestOption(t *testing.T) {
	client := servertest.New(t, newRouter())
	resp := client.GET("/secret", servertest.WithBearerToken("t0ken")).AssertStatus(http.StatusOK)
	got := servertest.DecodeData[user](t, resp)
	if got.ID != "me" {
		t.Errorf("expected authorized response, got %+v", got)
	}
}

func TestClient_DefaultHeader(t *testing.T) {
	client := servertest.New(t, newRouter())
	client.Header.Set("Authorization", "Bearer t0ken")
	client.GET("/secret").AssertStatus(http.StatusOK)
}
