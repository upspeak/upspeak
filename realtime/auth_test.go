package realtime

import (
	"net/http/httptest"
	"testing"
)

func TestAllowAllAuthenticator_PermitsEveryRequest(t *testing.T) {
	var auth Authenticator = allowAllAuthenticator{}
	req := httptest.NewRequest("GET", "/api/v1/ws", nil)

	identity, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if identity != "local" {
		t.Fatalf("expected identity %q, got %q", "local", identity)
	}
}
