package server

import (
	"reflect"
	"sync"
)

type GeneratedRouteMeta struct {
	PackagePath       string
	OperationID       string
	Method            string
	Path              string
	InputType         string
	OutputType        string
	InputTypeKey      string
	OutputTypeKey     string
	HasGeneratedCodec bool
}

var generatedManifestRegistry struct {
	mu     sync.RWMutex
	routes []GeneratedRouteMeta
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

	return append([]GeneratedRouteMeta(nil), generatedManifestRegistry.routes...)
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
