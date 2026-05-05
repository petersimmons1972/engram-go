package netutil

import (
	"testing"
)

// TestValidateUpstreamURLRejectsPrivateIPs verifies that ValidateUpstreamURL
// rejects URLs whose hostnames resolve to private/reserved IP addresses (#549).
func TestValidateUpstreamURLRejectsPrivateIPs(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		desc    string
	}{
		{
			name:    "localhost literal IP",
			url:     "http://127.0.0.1:9200",
			wantErr: true,
			desc:    "loopback should be rejected (#549)",
		},
		{
			name:    "localhost IPv6",
			url:     "http://[::1]:9200",
			wantErr: true,
			desc:    "IPv6 loopback should be rejected (#549)",
		},
		{
			name:    "RFC1918 10.0",
			url:     "http://10.0.0.1:9200",
			wantErr: true,
			desc:    "RFC1918 10.0.0.0/8 should be rejected",
		},
		{
			name:    "RFC1918 172.16",
			url:     "http://172.16.0.1:9200",
			wantErr: true,
			desc:    "RFC1918 172.16.0.0/12 should be rejected",
		},
		{
			name:    "RFC1918 192.168",
			url:     "http://192.168.1.1:9200",
			wantErr: true,
			desc:    "RFC1918 192.168.0.0/16 should be rejected",
		},
		{
			name:    "link-local",
			url:     "http://169.254.1.1:9200",
			wantErr: true,
			desc:    "link-local should be rejected (#549)",
		},
		{
			name:    "valid public literal IP",
			url:     "http://8.8.8.8:9200",
			wantErr: false,
			desc:    "public literal IPs should be accepted",
		},
		{
			name:    "invalid scheme",
			url:     "ftp://8.8.8.8:9200",
			wantErr: true,
			desc:    "non-HTTP schemes should be rejected",
		},
		{
			name:    "invalid URL",
			url:     "not a url",
			wantErr: true,
			desc:    "malformed URLs should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUpstreamURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("%s: ValidateUpstreamURL(%q) error = %v, wantErr = %v",
					tt.desc, tt.url, err, tt.wantErr)
			}
		})
	}
}
