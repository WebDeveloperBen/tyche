// Package servergen generates zero-reflection transport codecs (request
// binding, validation, and response serialization) for typed server routes by
// analysing server.Register calls with go/packages. The codecs are an
// optimization: routes bind and serialize via a reflection fallback without
// them, so running servergen is never required — it just removes reflection
// from the hot path. Types declared in package main are keyed to match Go's
// runtime reflection, so single-file main.go apps work too.
package servergen
