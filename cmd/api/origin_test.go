package main

import "testing"

// The CORS origin is compared byte for byte against the browser's Origin header,
// and a mismatch fails silently (server logs 200, browser discards the response),
// so each of these shapes is worth pinning down.
func TestNormalizeOrigin(t *testing.T) {
	cases := map[string]string{
		// Zerops' ${..._zeropsSubdomain} form: bare host, no scheme.
		"frontend-2e5c-3000.prg1.zerops.app": "https://frontend-2e5c-3000.prg1.zerops.app",
		// Already a full URL: left alone.
		"https://frontend-2e5c-3000.prg1.zerops.app": "https://frontend-2e5c-3000.prg1.zerops.app",
		// Trailing slash is not part of an Origin header.
		"https://example.com/": "https://example.com",
		// Local development must stay on http.
		"http://localhost:3000":  "http://localhost:3000",
		"http://localhost:3000/": "http://localhost:3000",
		// Empty stays empty so requireConfig still catches an unset value.
		"": "",
	}
	for in, want := range cases {
		if got := normalizeOrigin(in); got != want {
			t.Errorf("normalizeOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}
