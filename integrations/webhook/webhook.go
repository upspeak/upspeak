// Package webhook implements a connection-less core.Adapter that collects a
// single URL into one IngestItem. It is the proving adapter for the A2 registry
// + ingest pipeline. Identity, filtering, and persistence are the pipeline's
// job; this adapter only fetches and shapes content.
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/upspeak/upspeak/core"
)

// maxBodyBytes bounds a fetched body to protect memory.
const maxBodyBytes = 8 << 20 // 8 MiB

// Adapter fetches a URL and emits one IngestItem.
type Adapter struct {
	client *http.Client
}

// New creates a webhook adapter with a bounded HTTP client.
func New() *Adapter {
	return &Adapter{client: &http.Client{Timeout: 30 * time.Second}}
}

// Type identifies the connector.
func (a *Adapter) Type() core.ConnectorType { return core.ConnectorWebhook }

// Capabilities declares collect-only with no Connection required.
func (a *Adapter) Capabilities() core.AdapterCapabilities {
	return core.AdapterCapabilities{Collect: true, RequiresConnection: false}
}

// ConnectionSchema is empty: webhook is connection-less.
func (a *Adapter) ConnectionSchema() core.ConnectionSchema { return core.ConnectionSchema{} }

// ValidateConnectionConfig accepts anything: there is no Connection.
func (a *Adapter) ValidateConnectionConfig(map[string]any) error { return nil }

// ValidateSourceConfig requires a non-empty url field.
func (a *Adapter) ValidateSourceConfig(cfg map[string]any) error {
	if u, _ := cfg["url"].(string); u == "" {
		return errors.New("webhook source requires config.url")
	}
	return nil
}

// ValidateSinkConfig rejects all config: webhook is collect-only.
func (a *Adapter) ValidateSinkConfig(map[string]any) error {
	return errors.New("webhook connector does not support publishing")
}

// TestConnection is a no-op: there is no Connection to test.
func (a *Adapter) TestConnection(context.Context, *core.Connection) error { return nil }

// Collect fetches req.Source.Config["url"] and returns one IngestItem. For
// one-shot jobs the runner supplies an ephemeral Source carrying the URL.
func (a *Adapter) Collect(ctx context.Context, req core.CollectRequest) (*core.IngestBatch, error) {
	if req.Source == nil {
		return nil, errors.New("webhook collect requires a source")
	}
	url, _ := req.Source.Config["url"].(string)
	if url == "" {
		return nil, errors.New("webhook source missing config.url")
	}
	contentType, _ := req.Source.Config["content_type"].(string)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", redactURL(url), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch %s: unexpected status %d", redactURL(url), resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read body from %s: %w", redactURL(url), err)
	}
	if contentType == "" {
		contentType = resp.Header.Get("Content-Type")
	}
	if contentType == "" {
		contentType = "text/plain"
	}

	// Node.Body is json.RawMessage; wrap the fetched content as a JSON string
	// so callers can unmarshal it predictably without knowing the raw format.
	bodyJSON, err := json.Marshal(string(raw))
	if err != nil {
		return nil, fmt.Errorf("encode body: %w", err)
	}
	urlJSON, _ := json.Marshal(url)

	item := core.IngestItem{
		ExternalID: url,
		Node: &core.Node{
			Type:        "webpage",
			Subject:     url,
			ContentType: contentType,
			Body:        bodyJSON,
			Metadata:    []core.Metadata{{Key: "source_url", Value: urlJSON}},
		},
	}
	return &core.IngestBatch{Items: []core.IngestItem{item}}, nil
}

// redactURL returns a log-safe form of a URL, keeping only scheme and host so
// credentials embedded in userinfo, the path (e.g. Slack/Discord webhook
// tokens), or query parameters are never surfaced in error messages (which
// persist into the job's stored error).
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[redacted url]"
	}
	return u.Scheme + "://" + u.Host
}

// Compile-time contract checks.
var (
	_ core.Adapter   = (*Adapter)(nil)
	_ core.Collector = (*Adapter)(nil)
)
