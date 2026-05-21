package netutil

import (
	"net/http"
	"testing"
)

func TestValidateOutboundHTTPURLPublicOnlyRejectsUnsafeHosts(t *testing.T) {
	tests := []string{
		"http://127.0.0.1:8080",
		"https://localhost:8080",
		"https://10.0.0.1/hook",
		"https://metadata.google.internal/computeMetadata/v1",
		"http://api.example.com/v1",
	}

	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			if err := ValidateOutboundHTTPURL(rawURL, "webhook_url", true); err == nil {
				t.Fatal("ValidateOutboundHTTPURL returned nil error, want rejection")
			}
		})
	}
}

func TestValidateOutboundHTTPURLAllowsPublicHTTPS(t *testing.T) {
	if err := ValidateOutboundHTTPURL("https://api.example.com/v1", "base_url", true); err != nil {
		t.Fatalf("ValidateOutboundHTTPURL returned error: %v", err)
	}
}

func TestValidateOutboundHTTPURLAllowsLocalHTTPWhenPublicOnlyDisabled(t *testing.T) {
	if err := ValidateOutboundHTTPURL("http://127.0.0.1:8080", "base_url", false); err != nil {
		t.Fatalf("ValidateOutboundHTTPURL returned error: %v", err)
	}
}

func TestPublicOnlyHTTPClientRejectsUnsafeRedirect(t *testing.T) {
	client := PublicOnlyHTTPClient(0)
	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect is nil")
	}

	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1/metadata", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	if err := client.CheckRedirect(req, []*http.Request{{}}); err == nil {
		t.Fatal("CheckRedirect returned nil error, want unsafe redirect rejection")
	}
}

func TestPublicOnlyHTTPClientAllowsPublicHTTPSRedirect(t *testing.T) {
	client := PublicOnlyHTTPClient(0)
	req, err := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	if err := client.CheckRedirect(req, []*http.Request{{}}); err != nil {
		t.Fatalf("CheckRedirect returned error: %v", err)
	}
}
