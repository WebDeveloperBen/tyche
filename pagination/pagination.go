package pagination

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

var (
	ErrInvalidCursor      = errors.New("invalid cursor")
	ErrInvalidLimit       = errors.New("invalid limit")
	ErrDuplicateParameter = errors.New("duplicate parameter")
)

type Params struct {
	Limit  int    `query:"limit,omitempty"`
	Cursor string `query:"cursor,omitempty"`
}

type PageInfo struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type Config struct {
	DefaultLimit int
	MaxLimit     int
}

func DefaultConfig() Config {
	return Config{DefaultLimit: DefaultLimit, MaxLimit: MaxLimit}
}

func (c Config) Parse(values url.Values) (Params, error) {
	return Parse(values, c)
}

func Parse(values url.Values, config Config) (Params, error) {
	for _, name := range []string{"limit", "cursor"} {
		if len(values[name]) > 1 {
			return Params{}, &DuplicateParameterError{Name: name}
		}
	}
	params := Params{Cursor: values.Get("cursor")}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return Params{}, &InvalidLimitError{Value: raw, Reason: "must be an integer"}
		}
		if limit == 0 {
			return Params{}, &InvalidLimitError{Value: raw, Reason: "must be at least 1"}
		}
		params.Limit = limit
	}
	return config.Normalize(params)
}

func (c Config) Normalize(params Params) (Params, error) {
	config, err := c.normalized()
	if err != nil {
		return Params{}, err
	}
	if params.Cursor != "" && strings.TrimSpace(params.Cursor) == "" {
		return Params{}, &InvalidCursorError{Value: params.Cursor}
	}
	if params.Limit == 0 {
		params.Limit = config.DefaultLimit
	}
	if params.Limit < 1 || params.Limit > config.MaxLimit {
		return Params{}, &InvalidLimitError{
			Value:  strconv.Itoa(params.Limit),
			Reason: fmt.Sprintf("must be between 1 and %d", config.MaxLimit),
		}
	}
	return params, nil
}

func (c Config) normalized() (Config, error) {
	config := c
	if config.DefaultLimit == 0 {
		config.DefaultLimit = DefaultLimit
	}
	if config.MaxLimit == 0 {
		config.MaxLimit = MaxLimit
	}
	if config.DefaultLimit < 1 || config.DefaultLimit > config.MaxLimit {
		return Config{}, fmt.Errorf("pagination default limit %d must be between 1 and maximum %d", config.DefaultLimit, config.MaxLimit)
	}
	return config, nil
}

type InvalidLimitError struct {
	Value  string
	Reason string
}

func (e *InvalidLimitError) Error() string {
	return fmt.Sprintf("invalid limit %q: %s", e.Value, e.Reason)
}

func (e *InvalidLimitError) Unwrap() error { return ErrInvalidLimit }

type DuplicateParameterError struct {
	Name string
}

func (e *DuplicateParameterError) Error() string {
	return fmt.Sprintf("duplicate pagination parameter %q", e.Name)
}

func (e *DuplicateParameterError) Unwrap() error { return ErrDuplicateParameter }

type InvalidCursorError struct {
	Value string
}

func (e *InvalidCursorError) Error() string {
	return fmt.Sprintf("invalid cursor %q", e.Value)
}

func (e *InvalidCursorError) Unwrap() error { return ErrInvalidCursor }
