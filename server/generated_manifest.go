package server

import (
	"reflect"
	"sync"
)

type GeneratedRouteMeta struct {
	PackagePath          string
	OperationID          string
	Method               string
	Path                 string
	InputType            string
	OutputType           string
	InputTypeKey         string
	OutputTypeKey        string
	ResponseContentTypes []string
	HasGeneratedCodec    bool
}

var generatedManifestRegistry struct {
	routes []GeneratedRouteMeta
	mu     sync.RWMutex
}

func RegisterGeneratedManifest(routes ...GeneratedRouteMeta) {
	generatedManifestRegistry.mu.Lock()
	defer generatedManifestRegistry.mu.Unlock()

	existing := make(map[string]struct{}, len(generatedManifestRegistry.routes))
	for _, route := range generatedManifestRegistry.routes {
		existing[generatedRouteIdentity(route)] = struct{}{}
	}

	for _, route := range routes {
		identity := generatedRouteIdentity(route)
		if _, ok := existing[identity]; ok {
			panic("generated manifest already registered for route: " + identity)
		}
		existing[identity] = struct{}{}
	}

	generatedManifestRegistry.routes = append(generatedManifestRegistry.routes, routes...)
}

func GeneratedRouteManifest() []GeneratedRouteMeta {
	generatedManifestRegistry.mu.RLock()
	defer generatedManifestRegistry.mu.RUnlock()

	out := make([]GeneratedRouteMeta, len(generatedManifestRegistry.routes))
	for i, route := range generatedManifestRegistry.routes {
		out[i] = cloneGeneratedRouteMeta(route)
	}
	return out
}

func cloneGeneratedRouteMeta(route GeneratedRouteMeta) GeneratedRouteMeta {
	route.ResponseContentTypes = append([]string(nil), route.ResponseContentTypes...)
	return route
}

func generatedRouteIdentity(route GeneratedRouteMeta) string {
	return route.PackagePath + "|" + route.OperationID + "|" + route.Method + "|" + route.Path + "|" + route.InputTypeKey + "|" + route.OutputTypeKey
}

func GeneratedTypeKey(t reflect.Type) string {
	base := indirectType(t)
	if base.PkgPath() != "" && base.Name() != "" {
		return base.PkgPath() + "." + base.Name()
	}
	return base.String()
}
