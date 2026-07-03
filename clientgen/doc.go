// Package clientgen generates a self-contained, dependency-free typed Go client
// from a tyche-produced OpenAPI 3.x document.
//
// The generated module imports only the standard library: it emits its own copy
// of the request/response types (option "B" — no dependency on the server
// package), one method per operation, and bakes in tyche's conventions — the
// {"data": …} success envelope and the application/problem+json error shape
// (surfaced as a typed *APIError).
//
// Types are recovered from the document by walking each operation's (inlined)
// schemas and deduplicating by structural identity, so shapes that also appear
// as named entries in components.schemas collapse to a single clean Go type.
// Set Options.TypeNamingStrategy to TypeNamingOperationScoped when distinct
// operations should generate distinct Go types even if their schemas are
// structurally identical.
//
// Typical use is via the `tyche client` command; Generate is the
// programmatic entry point:
//
//	doc, _ := clientgen.ParseDocument(specJSON)
//	res, _ := clientgen.Generate(doc, clientgen.Options{
//		Module: "github.com/yourco/gateway/client",
//	})
//	for _, f := range res.Files { os.WriteFile(filepath.Join(outDir, f.Name), f.Content, 0o644) }
//
// Server-Sent Events operations (text/event-stream) generate a streaming method
// returning a typed *Stream[Event], iterated with the scanner pattern. Event
// name, ID, and retry metadata are available from the most recent event.
package clientgen
