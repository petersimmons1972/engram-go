package netutil

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

type lookupIPAddrFunc func(context.Context, string) ([]net.IPAddr, error)

func (f lookupIPAddrFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

func (f dialContextFunc) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return f(ctx, network, addr)
}

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		name string
		ip   string
		want bool
	}{
		// IPv4 loopback
		{"ipv4 loopback 127.0.0.1", "127.0.0.1", true},
		{"ipv4 loopback 127.255.255.255", "127.255.255.255", true},

		// RFC1918
		{"rfc1918 10.0.0.1", "10.0.0.1", true},
		{"rfc1918 10.255.255.255", "10.255.255.255", true},
		{"rfc1918 172.16.0.1", "172.16.0.1", true},
		{"rfc1918 172.31.255.255", "172.31.255.255", true},
		{"rfc1918 192.168.1.1", "192.168.1.1", true},
		{"rfc1918 192.168.255.255", "192.168.255.255", true},

		// Link-local
		{"link-local ipv4 169.254.1.1", "169.254.1.1", true},

		// Carrier-grade NAT
		{"cgnat 100.64.0.1", "100.64.0.1", true},

		// Benchmark range (RFC 2544)
		{"benchmark 198.18.0.1", "198.18.0.1", true},

		// Reserved
		{"reserved 240.0.0.1", "240.0.0.1", true},

		// "This" network
		{"this-network 0.0.0.1", "0.0.0.1", true},

		// Public IPv4 — must be false
		{"public 8.8.8.8", "8.8.8.8", false},
		{"public 1.1.1.1", "1.1.1.1", false},
		{"public 203.0.113.1", "203.0.113.1", false},

		// IPv6 loopback
		{"ipv6 loopback ::1", "::1", true},

		// IPv6 link-local
		{"ipv6 link-local fe80::1", "fe80::1", true},
		{"ipv6 link-local fe80::dead:beef", "fe80::dead:beef", true},

		// IPv6 ULA
		{"ipv6 ULA fc00::1", "fc00::1", true},
		{"ipv6 ULA fd00::1", "fd00::1", true},

		// Public IPv6 — must be false
		{"public ipv6 2001:db8::1", "2001:db8::1", false},
		{"public ipv6 2606:4700::1", "2606:4700::1", false},

		// IPv4-mapped IPv6 — must match their IPv4 classification
		{"ipv4-mapped loopback ::ffff:127.0.0.1", "::ffff:127.0.0.1", true},
		{"ipv4-mapped rfc1918 ::ffff:192.168.1.1", "::ffff:192.168.1.1", true},
		{"ipv4-mapped public ::ffff:8.8.8.8", "::ffff:8.8.8.8", false},

		// Invalid / non-IP input
		{"empty string", "", false},
		{"hostname", "example.com", false},
		{"garbage", "not-an-ip", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsPrivateIP(tc.ip)
			if got != tc.want {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestValidateUpstreamURL_S549(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		wantError bool
		errMsg    string
	}{
		// Literal IPs — private (should error)
		{"literal loopback", "http://127.0.0.1:9200", true, "private IP"},
		{"literal rfc1918 10", "http://10.0.0.1:8000", true, "private IP"},
		{"literal rfc1918 172", "http://172.16.0.1:8000", true, "private IP"},
		{"literal rfc1918 192", "http://192.168.1.1:8000", true, "private IP"},

		// Literal IPs — public (should pass)
		{"literal public 8.8.8.8", "http://8.8.8.8:53", false, ""},
		{"literal public 1.1.1.1", "http://1.1.1.1:53", false, ""},

		// Invalid URLs
		{"invalid scheme ftp", "ftp://example.com", true, "invalid scheme"},
		{"malformed url", "ht!tp://example.com", true, "invalid URL"},
		{"no hostname", "http://", true, "no hostname"},

		// Hostname without scheme
		{"no scheme", "example.com", true, "invalid scheme"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateUpstreamURL(tc.url)
			if tc.wantError && err == nil {
				t.Errorf("ValidateUpstreamURL(%q) = nil, wanted error", tc.url)
			}
			if !tc.wantError && err != nil {
				t.Errorf("ValidateUpstreamURL(%q) = %v, wanted no error", tc.url, err)
			}
			if tc.wantError && err != nil && tc.errMsg != "" && !strings.Contains(err.Error(), tc.errMsg) {
				t.Errorf("ValidateUpstreamURL(%q) error %q does not contain %q", tc.url, err, tc.errMsg)
			}
		})
	}
}

func TestValidateUpstreamURL_DialTimeReResolutionRejectsPrivateRebind(t *testing.T) {
	t.Parallel()

	var lookups atomic.Int32
	resolver := lookupIPAddrFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		switch call := lookups.Add(1); call {
		case 1:
			return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
		case 2:
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		default:
			return nil, errors.New("unexpected extra lookup")
		}
	})

	rawURL := "http://rebind.example.test:8080"
	if err := validateUpstreamURLWithResolver(rawURL, resolver); err != nil {
		t.Fatalf("validateUpstreamURLWithResolver() first lookup failed: %v", err)
	}

	var dials atomic.Int32
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: NewUpstreamDialContext(rawURL, SafeDialOptions{
				Resolver: resolver,
				Dialer: dialContextFunc(func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)
					return nil, errors.New("must reject before dialing")
				}),
				AllowPrivateConfiguredHost: false,
				ErrorPrefix:                "ollama_url",
			}),
		},
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL+"/v1/models", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	_, err = client.Do(req)
	if err == nil {
		t.Fatal("client.Do() error = nil, want private-IP rejection on second lookup")
	}
	if !strings.Contains(err.Error(), "private IP") {
		t.Fatalf("client.Do() error = %v, want private-IP rejection", err)
	}
	if got := dials.Load(); got != 0 {
		t.Fatalf("DialContext called %d times, want 0 after private-IP rebind rejection", got)
	}
	if got := lookups.Load(); got != 2 {
		t.Fatalf("resolver lookups = %d, want validate lookup + dial-time lookup", got)
	}
}

func TestValidateUpstreamURL_DialTimeResolutionUsesLiteralPublicIP(t *testing.T) {
	t.Parallel()

	resolver := lookupIPAddrFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("203.0.113.10")}}, nil
	})

	rawURL := "http://public.example.test:8080"
	if err := validateUpstreamURLWithResolver(rawURL, resolver); err != nil {
		t.Fatalf("validateUpstreamURLWithResolver() error = %v", err)
	}

	var dialedAddr atomic.Value
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: NewUpstreamDialContext(rawURL, SafeDialOptions{
				Resolver: resolver,
				Dialer: dialContextFunc(func(_ context.Context, network, addr string) (net.Conn, error) {
					dialedAddr.Store(addr)
					serverConn, clientConn := net.Pipe()
					go func() {
						defer serverConn.Close()
						reader := bufio.NewReader(serverConn)
						for {
							line, readErr := reader.ReadString('\n')
							if readErr != nil {
								return
							}
							if line == "\r\n" {
								break
							}
						}
						_, _ = io.WriteString(serverConn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
					}()
					return clientConn, nil
				}),
				AllowPrivateConfiguredHost: false,
				ErrorPrefix:                "ollama_url",
			}),
		},
	}

	resp, err := client.Get(rawURL + "/v1/models")
	if err != nil {
		t.Fatalf("client.Get() error = %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("response body = %q, want ok", body)
	}
	if got, _ := dialedAddr.Load().(string); got != "203.0.113.10:8080" {
		t.Fatalf("dialed addr = %q, want literal public IP", got)
	}
}

func TestValidateUpstreamURL_HostnameResolvingPrivateStillFailsOnFirstLookup(t *testing.T) {
	t.Parallel()

	resolver := lookupIPAddrFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})

	err := validateUpstreamURLWithResolver("http://private.example.test:8080", resolver)
	if err == nil {
		t.Fatal("validateUpstreamURLWithResolver() error = nil, want private hostname rejection")
	}
	if !strings.Contains(err.Error(), "private IP") {
		t.Fatalf("validateUpstreamURLWithResolver() error = %v, want private-IP rejection", err)
	}
}
