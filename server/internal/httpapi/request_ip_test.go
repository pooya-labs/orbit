package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestRequestIPUsesCloudflareGatewayAddressOnlyWhenValid(t *testing.T) {
	tests := []struct {
		name       string
		forwarded  string
		remoteAddr string
		want       string
	}{
		{name: "trusted IPv4 gateway header", forwarded: "198.51.100.24", remoteAddr: "172.18.0.5:8080", want: "198.51.100.24"},
		{name: "trusted IPv6 gateway header", forwarded: "2001:db8::24", remoteAddr: "172.18.0.5:8080", want: "2001:db8::24"},
		{name: "invalid gateway header falls back to peer", forwarded: "198.51.100.24, 203.0.113.9", remoteAddr: "172.18.0.5:8080", want: "172.18.0.5"},
		{name: "no gateway header falls back to peer", remoteAddr: "192.0.2.10:4321", want: "192.0.2.10"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/meta", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwarded != "" {
				req.Header.Set("X-ORBIT-Client-IP", tt.forwarded)
			}
			if got := requestIP(req); got != tt.want {
				t.Fatalf("requestIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
