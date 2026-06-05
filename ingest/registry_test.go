package ingest

import (
	"context"
	"testing"

	"github.com/upspeak/upspeak/core"
)

// stubAdapter is a minimal core.Adapter for registry tests.
type stubAdapter struct{ t core.ConnectorType }

func (s stubAdapter) Type() core.ConnectorType                       { return s.t }
func (s stubAdapter) Capabilities() core.AdapterCapabilities         { return core.AdapterCapabilities{} }
func (s stubAdapter) ConnectionSchema() core.ConnectionSchema        { return core.ConnectionSchema{} }
func (s stubAdapter) ValidateConnectionConfig(map[string]any) error  { return nil }
func (s stubAdapter) ValidateSourceConfig(map[string]any) error      { return nil }
func (s stubAdapter) ValidateSinkConfig(map[string]any) error        { return nil }
func (s stubAdapter) TestConnection(context.Context, *core.Connection) error { return nil }

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewRegistry()
	r.Register(stubAdapter{t: core.ConnectorWebhook})

	got, ok := r.AdapterFor(core.ConnectorWebhook)
	if !ok {
		t.Fatal("expected webhook adapter to be registered")
	}
	if got.Type() != core.ConnectorWebhook {
		t.Fatalf("got adapter type %q, want webhook", got.Type())
	}

	if _, ok := r.AdapterFor(core.ConnectorRSS); ok {
		t.Fatal("expected no adapter for rss")
	}
}

func TestRegistry_RegisterOverwrites(t *testing.T) {
	r := NewRegistry()
	r.Register(stubAdapter{t: core.ConnectorWebhook})
	r.Register(stubAdapter{t: core.ConnectorWebhook}) // last write wins, no panic
	if _, ok := r.AdapterFor(core.ConnectorWebhook); !ok {
		t.Fatal("expected webhook adapter after re-register")
	}
}
