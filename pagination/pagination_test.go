package pagination_test

import (
	"encoding/json"
	"errors"
	"net/url"
	"testing"

	"github.com/webdeveloperben/tyche/pagination"
)

func TestParseAppliesDefaultsAndLimits(t *testing.T) {
	params, err := pagination.Parse(url.Values{"cursor": {"opaque"}}, pagination.Config{DefaultLimit: 25, MaxLimit: 50})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if params.Limit != 25 || params.Cursor != "opaque" {
		t.Fatalf("params = %#v, want limit 25 and cursor opaque", params)
	}
}

func TestParseRejectsInvalidLimits(t *testing.T) {
	for name, values := range map[string]url.Values{
		"not an integer": {"limit": {"nope"}},
		"zero":           {"limit": {"0"}},
		"too large":      {"limit": {"51"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pagination.Parse(values, pagination.Config{DefaultLimit: 25, MaxLimit: 50})
			if !errors.Is(err, pagination.ErrInvalidLimit) {
				t.Fatalf("error = %v, want ErrInvalidLimit", err)
			}
		})
	}
}

func TestParseRejectsDuplicateControls(t *testing.T) {
	for name, values := range map[string]url.Values{
		"limit":  {"limit": {"1", "2"}},
		"cursor": {"cursor": {"one", "two"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pagination.Parse(values, pagination.Config{})
			if !errors.Is(err, pagination.ErrDuplicateParameter) {
				t.Fatalf("error = %v, want ErrDuplicateParameter", err)
			}
			var duplicate *pagination.DuplicateParameterError
			if !errors.As(err, &duplicate) || duplicate.Name != name {
				t.Fatalf("error = %v, want duplicate %q", err, name)
			}
		})
	}
}

func TestNormalizeRejectsWhitespaceCursor(t *testing.T) {
	_, err := (pagination.Config{}).Normalize(pagination.Params{Cursor: "  "})
	if !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("error = %v, want ErrInvalidCursor", err)
	}
}

func TestPageInfoMarshalsTransportShape(t *testing.T) {
	data, err := json.Marshal(pagination.PageInfo{NextCursor: "opaque", HasMore: true})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `{"next_cursor":"opaque","has_more":true}` {
		t.Fatalf("JSON = %s, want pagination page shape", data)
	}
}
