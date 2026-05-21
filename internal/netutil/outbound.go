package netutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultOutboundTimeout = 10 * time.Second

func ValidateOutboundHTTPURL(rawURL string, field string, publicOnly bool) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("%s must be a valid URL", field)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", field)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include host", field)
	}
	if parsed.User != nil {
		return fmt.Errorf("%s must not include user info", field)
	}
	if publicOnly && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use https", field)
	}
	if publicOnly && unsafeHostname(parsed.Hostname()) {
		return fmt.Errorf("%s must use a public host", field)
	}
	return nil
}

func PublicOnlyHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultOutboundTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	transport.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, addr := range ips {
			if !IsPublicIP(addr.IP) {
				continue
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		}
		return nil, fmt.Errorf("blocked outbound connection to non-public host %s", host)
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if err := ValidateOutboundHTTPURL(req.URL.String(), "redirect_url", true); err != nil {
				return err
			}
			return nil
		},
	}
}

func TimeoutHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultOutboundTimeout
	}
	return &http.Client{Timeout: timeout}
}

func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	return true
}

func unsafeHostname(host string) bool {
	host = strings.TrimSpace(strings.ToLower(strings.TrimSuffix(host, ".")))
	if host == "" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return !IsPublicIP(ip)
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	switch host {
	case "metadata.google.internal":
		return true
	default:
		return false
	}
}
