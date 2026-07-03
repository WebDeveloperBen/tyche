package config

import "github.com/webdeveloperben/tyche/clientgen"

func defaultClientOpts() clientgen.Options {
	return clientgen.Options{
		Module:             "client",
		GoVersion:          "1.22",
		TypeNamingStrategy: clientgen.TypeNamingStructural,
	}
}

func equalClientOpts(a, b clientgen.Options) bool {
	return a.Module == b.Module &&
		a.Package == b.Package &&
		a.GoVersion == b.GoVersion &&
		a.ClientName == b.ClientName &&
		a.TypeNamingStrategy == b.TypeNamingStrategy
}
