package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/upspeak/upspeak/core"
)

func TestAdapter_Collect_FetchesURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	a := New()
	src := &core.Source{Config: map[string]any{"url": srv.URL}}
	batch, err := a.Collect(context.Background(), core.CollectRequest{Source: src})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(batch.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(batch.Items))
	}
	item := batch.Items[0]
	if item.ExternalID != srv.URL {
		t.Fatalf("external id = %q, want %q", item.ExternalID, srv.URL)
	}
	var body string
	if err := json.Unmarshal(item.Node.Body, &body); err != nil {
		t.Fatalf("body is not a JSON string: %v", err)
	}
	if body != "hello world" {
		t.Fatalf("body = %q", body)
	}
}

func TestAdapter_Collect_MissingURL(t *testing.T) {
	a := New()
	if _, err := a.Collect(context.Background(), core.CollectRequest{Source: &core.Source{Config: map[string]any{}}}); err == nil {
		t.Fatal("expected error for missing url")
	}
}

func TestAdapter_Collect_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := New()
	src := &core.Source{Config: map[string]any{"url": srv.URL}}
	if _, err := a.Collect(context.Background(), core.CollectRequest{Source: src}); err == nil {
		t.Fatal("expected error for non-2xx response")
	}
}

func TestAdapter_Contract(t *testing.T) {
	a := New()
	if a.Type() != core.ConnectorWebhook {
		t.Fatalf("type = %q", a.Type())
	}
	if !a.Capabilities().Collect || a.Capabilities().RequiresConnection {
		t.Fatalf("unexpected capabilities: %+v", a.Capabilities())
	}
	if err := a.ValidateSourceConfig(map[string]any{}); err == nil {
		t.Fatal("expected source config to require url")
	}
	if err := a.ValidateSinkConfig(map[string]any{}); err == nil {
		t.Fatal("webhook must reject sink config (collect-only)")
	}
}
