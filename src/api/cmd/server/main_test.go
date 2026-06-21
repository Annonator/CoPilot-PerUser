package main

import (
	"net/http"
	"testing"
	"time"
)

func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	handler := http.NewServeMux()

	server := newHTTPServer(":9090", handler)

	if server.Addr != ":9090" {
		t.Fatalf("Addr = %q, want :9090", server.Addr)
	}
	if server.Handler != handler {
		t.Fatal("Handler was not preserved")
	}

	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{name: "ReadHeaderTimeout", got: server.ReadHeaderTimeout, want: 5 * time.Second},
		{name: "ReadTimeout", got: server.ReadTimeout, want: 15 * time.Second},
		{name: "WriteTimeout", got: server.WriteTimeout, want: 60 * time.Second},
		{name: "IdleTimeout", got: server.IdleTimeout, want: 120 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %s, want %s", tt.name, tt.got, tt.want)
			}
		})
	}
}
