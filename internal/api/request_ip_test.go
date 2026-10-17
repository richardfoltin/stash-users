package api

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRequestIP(t *testing.T) {
	_, trustedNet, _ := net.ParseCIDR("198.51.100.0/24")
	trustedNets := []net.IPNet{*trustedNet}

	tests := []struct {
		name         string
		remoteAddr   string
		forwardedFor []string
		want         string
		wantLocal    bool
	}{
		{"direct local client", "192.168.1.20:50000", nil, "192.168.1.20", true},
		{"direct public client", "203.0.113.7:50000", nil, "203.0.113.7", false},
		{"local client through proxy", "127.0.0.1:50000", []string{"192.168.1.20"}, "192.168.1.20", true},
		{"public client through proxy", "127.0.0.1:50000", []string{"203.0.113.7"}, "203.0.113.7", false},
		{"spoofed local address through appending proxy", "127.0.0.1:50000", []string{"192.168.1.5, 203.0.113.7"}, "203.0.113.7", false},
		{"spoofed local address in separate header", "127.0.0.1:50000", []string{"192.168.1.5", "203.0.113.7"}, "203.0.113.7", false},
		{"public client through trusted proxies", "198.51.100.1:50000", []string{"192.168.1.5,203.0.113.7, 198.51.100.2"}, "203.0.113.7", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			for _, v := range tt.forwardedFor {
				r.Header.Add("X-Forwarded-For", v)
			}

			got, err := getRequestIP(r, trustedNets, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
			assert.Equal(t, tt.wantLocal, isLocalIP(got))
		})
	}
}

func TestGetRequestIP_Rejected(t *testing.T) {
	tests := []struct {
		name         string
		remoteAddr   string
		forwardedFor string
	}{
		{"forwarded by public client", "203.0.113.7:50000", "192.168.1.5"},
		{"invalid forwarded address", "127.0.0.1:50000", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			r.Header.Set("X-Forwarded-For", tt.forwardedFor)

			_, err := getRequestIP(r, nil, nil)
			assert.Error(t, err)
		})
	}
}
