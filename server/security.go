package server

import "github.com/webdeveloperben/tyche/server/openapi"

// SecurityScheme is an alias for [openapi.SecurityScheme], re-exported so most
// applications can declare authentication without importing the openapi
// package directly. Prefer the constructors [APIKeyScheme], [BearerScheme], and
// [BasicScheme] for the common cases.
type SecurityScheme = openapi.SecurityScheme

// AddSecurityScheme registers a named security scheme in the OpenAPI document's
// components. Like other OpenAPI registration it should be called during setup,
// before the document is served concurrently. Operations reference it by name
// through [Operation.Security]:
//
//	router.AddSecurityScheme("bearer", server.BearerScheme("JWT"))
//
//	server.Register(api, op, handler) // op.Security: []server.SecurityRequirement{{"bearer": {}}}
func (r *Router) AddSecurityScheme(name string, scheme *SecurityScheme) {
	r.OpenAPI().AddSecurityScheme(name, scheme)
	r.invalidateOpenAPICache()
}

// APIKeyScheme describes an API key carried in a header, query parameter, or
// cookie. in must be one of "header", "query", or "cookie".
func APIKeyScheme(parameterName, in string) *SecurityScheme {
	return &SecurityScheme{
		Type: "apiKey",
		Name: parameterName,
		In:   in,
	}
}

// BearerScheme describes an HTTP bearer-token scheme (the Authorization:
// Bearer <token> header). bearerFormat is an optional, informational hint such
// as "JWT"; pass "" to omit it.
func BearerScheme(bearerFormat string) *SecurityScheme {
	return &SecurityScheme{
		Type:         "http",
		Scheme:       "bearer",
		BearerFormat: bearerFormat,
	}
}

// BasicScheme describes HTTP Basic authentication.
func BasicScheme() *SecurityScheme {
	return &SecurityScheme{
		Type:   "http",
		Scheme: "basic",
	}
}

// normalizeSecurityRequirements converts nil scope slices to empty slices so the
// emitted OpenAPI document uses an empty array (valid) rather than null for
// schemes that take no scopes.
func normalizeSecurityRequirements(reqs []SecurityRequirement) []map[string][]string {
	if reqs == nil {
		return nil
	}
	out := make([]map[string][]string, len(reqs))
	for i, r := range reqs {
		m := make(map[string][]string, len(r))
		for k, v := range r {
			if v == nil {
				v = []string{}
			}
			m[k] = v
		}
		out[i] = m
	}
	return out
}
