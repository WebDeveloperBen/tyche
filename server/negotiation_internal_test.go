package server

import "testing"

func TestAcceptsMediaType(t *testing.T) {
	tests := []struct {
		name      string
		accept    string
		mediaType string
		want      bool
	}{
		{name: "empty accepts", accept: "", mediaType: "application/json", want: true},
		{name: "exact accepts", accept: "application/json", mediaType: "application/json", want: true},
		{name: "type wildcard accepts", accept: "application/*", mediaType: "application/json", want: true},
		{name: "any wildcard accepts", accept: "*/*", mediaType: "application/json", want: true},
		{name: "specific q zero rejects despite wildcard", accept: "application/json;q=0, */*;q=1", mediaType: "application/json", want: false},
		{name: "type wildcard q zero rejects despite any wildcard", accept: "application/*;q=0, */*;q=1", mediaType: "application/json", want: false},
		{name: "vendor json suffix wildcard accepts", accept: "application/*+json", mediaType: "application/vnd.tyche+json", want: true},
		{name: "json is not suffix json", accept: "application/*+json", mediaType: "application/json", want: false},
		{name: "malformed range ignored", accept: "bad range, application/json", mediaType: "application/json", want: true},
		{name: "only malformed range rejects", accept: "bad range", mediaType: "application/json", want: false},
		{name: "different type rejects", accept: "text/plain", mediaType: "application/json", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acceptsMediaType(tt.accept, tt.mediaType); got != tt.want {
				t.Fatalf("acceptsMediaType(%q, %q) = %v, want %v", tt.accept, tt.mediaType, got, tt.want)
			}
		})
	}
}
