package daemon

import "testing"

func TestRuntimeAPIURL(t *testing.T) {
	tests := map[string]string{
		"127.0.0.1:27892":       "http://127.0.0.1:27892",
		"0.0.0.0:27892":         "http://127.0.0.1:27892",
		"[::]:27892":            "http://127.0.0.1:27892",
		":27892":                "http://127.0.0.1:27892",
		"http://localhost:123/": "http://localhost:123",
	}
	for input, want := range tests {
		t.Run(input, func(t *testing.T) {
			if got := RuntimeAPIURL(input); got != want {
				t.Fatalf("RuntimeAPIURL(%q)=%q, want %q", input, got, want)
			}
		})
	}
}
