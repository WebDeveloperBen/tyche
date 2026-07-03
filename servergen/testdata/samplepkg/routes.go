package samplepkg

import (
	"context"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/webdeveloperben/tyche/server"
)

type Store interface {
	Get(context.Context, string) (string, error)
}

type GetThingRequest struct {
	ID string `path:"id"`
}

type GetThingResponse struct {
	Body struct {
		ID string `json:"id"`
	} `body:"true"`
}

type CreateThingRequest struct {
	ID   string `path:"id"`
	Body struct {
		Name    string   `json:"name" validate:"min=2,max=10"`
		Kind    string   `json:"kind" validate:"oneof=a b"`
		Aliases []string `json:"aliases" validate:"items.min=2"`
		Meta    struct {
			Code string `json:"code" validate:"pattern=^[A-Z]{2}$"`
		} `json:"meta"`
		Children []struct {
			Code string `json:"code" validate:"pattern=^[A-Z]{2}$"`
		} `json:"children"`
	} `body:"true"`
}

type CreateThingResponse struct {
	Status int `status:"201"`
	Body   struct {
		ID string `json:"id"`
	} `body:"true"`
}

type BulkCreateThingRequest struct {
	Body []struct {
		Code string `json:"code" validate:"pattern=^[A-Z]{2}$"`
	} `body:"true"`
}

type BulkCreateThingResponse struct {
	Body []struct {
		ID string `json:"id"`
	} `body:"true"`
}

type UnsupportedThingRequest struct {
	When time.Time `query:"when"`
}

type UnsupportedThingResponse struct {
	Body struct {
		OK bool `json:"ok"`
	} `body:"true"`
}

type FlatThingRequest struct {
	Body struct {
		Name  string `json:"name" validate:"min=2,max=10"`
		Kind  string `json:"kind" validate:"oneof=a b"`
		Code  string `json:"code" validate:"pattern=^[A-Z]{2}$"`
		Email string `json:"email" validate:"email"`
	} `body:"true"`
}

type FlatThingResponse struct {
	Body struct {
		OK bool `json:"ok"`
	} `body:"true"`
}

type UploadThingRequest struct {
	Title string                `form:"title"`
	File  *multipart.FileHeader `file:"file"`
}

type UploadThingResponse struct {
	Body struct {
		OK bool `json:"ok"`
	} `body:"true"`
}

func RegisterRoutes(grp *server.APIGroup, store Store) {
	server.Register(grp, server.Operation{
		OperationID: "get-thing",
		Method:      http.MethodGet,
		Path:        "/things/:id",
		Summary:     "Get thing",
	}, func(ctx context.Context, input *GetThingRequest) (*GetThingResponse, error) {
		value, err := store.Get(ctx, input.ID)
		if err != nil {
			return nil, err
		}

		out := &GetThingResponse{}
		out.Body.ID = value
		return out, nil
	})

	server.Register(grp, server.Operation{
		OperationID: "create-thing",
		Method:      http.MethodPost,
		Path:        "/things/:id",
		Summary:     "Create thing",
	}, func(ctx context.Context, input *CreateThingRequest) (*CreateThingResponse, error) {
		out := &CreateThingResponse{Status: http.StatusCreated}
		out.Body.ID = input.ID + "-" + input.Body.Kind
		return out, nil
	})

	server.Register(grp, server.Operation{
		OperationID: "bulk-create-thing",
		Method:      http.MethodPost,
		Path:        "/things/bulk",
		Summary:     "Bulk create thing",
	}, func(ctx context.Context, input *BulkCreateThingRequest) (*BulkCreateThingResponse, error) {
		out := &BulkCreateThingResponse{Body: make([]struct {
			ID string `json:"id"`
		}, len(input.Body))}
		for i := range input.Body {
			out.Body[i].ID = input.Body[i].Code
		}
		return out, nil
	})

	server.Register(grp, server.Operation{
		OperationID: "unsupported-thing",
		Method:      http.MethodGet,
		Path:        "/things/unsupported",
		Summary:     "Unsupported thing",
	}, func(ctx context.Context, input *UnsupportedThingRequest) (*UnsupportedThingResponse, error) {
		out := &UnsupportedThingResponse{}
		out.Body.OK = !input.When.IsZero()
		return out, nil
	})

	server.Register(grp, server.Operation{
		OperationID: "flat-thing",
		Method:      http.MethodPost,
		Path:        "/things/flat",
		Summary:     "Flat thing",
	}, func(ctx context.Context, input *FlatThingRequest) (*FlatThingResponse, error) {
		out := &FlatThingResponse{}
		out.Body.OK = input.Body.Kind != ""
		return out, nil
	})

	server.Register(grp, server.Operation{
		OperationID: "upload-thing",
		Method:      http.MethodPost,
		Path:        "/things/upload",
		Summary:     "Upload thing",
	}, func(ctx context.Context, input *UploadThingRequest) (*UploadThingResponse, error) {
		out := &UploadThingResponse{}
		out.Body.OK = input.Title != "" && input.File != nil
		return out, nil
	})
}
