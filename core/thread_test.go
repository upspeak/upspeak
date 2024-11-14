package core

import (
	"testing"

	"github.com/rs/xid"
)

func compareContent(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestNewThread(t *testing.T) {
	tests := []struct {
		name   string
		body   ThreadBody
		author string
	}{
		{
			name: "Basic thread creation",
			body: ThreadBody{
				Title:       "Test Thread",
				ContentType: "text/plain",
				Content:     map[string]any{"text": "This is a test thread"},
			},
			author: "@alice:example.com",
		},
		{
			name: "Basic thread creation with nil content",
			body: ThreadBody{
				Title:       "Test Thread with no added content",
				ContentType: "",
				Content:     nil,
			},
			author: "@bob:example.com",
		},
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread := NewThread(tt.body, tt.author)
			if thread.Metadata.Author != tt.author {
				t.Errorf("expected author %s, got %s", tt.author, thread.Metadata.Author)
			}
			if thread.Metadata.Version != 1 {
				t.Errorf("expected version 1, got %d", thread.Metadata.Version)
			}
			if thread.Body.Title != tt.body.Title {
				t.Errorf("expected title %s, got %s", tt.body.Title, thread.Body.Title)
			}
			if thread.Body.ContentType != tt.body.ContentType {
				t.Errorf("expected content type %s, got %s", tt.body.ContentType, thread.Body.ContentType)
			}
			if !compareContent(thread.Body.Content, tt.body.Content) {
				t.Errorf("expected content %v, got %v", tt.body.Content, thread.Body.Content)
			}
			if thread.Kind != KindThread {
				t.Errorf("expected kind %s, got %s", KindThread, thread.Kind)
			}
		})
	}
}

func TestSetProps(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "Set key1", key: "key1", value: "value1"},
		{name: "Set key2", key: "key2", value: "value2"},
		// Add more test cases here
	}

	body := ThreadBody{
		Title:       "Test Thread",
		ContentType: "text/plain",
		Content:     map[string]any{"text": "This is a test thread"},
	}
	author := "@alice:example.com"
	thread := NewThread(body, author)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread.SetProps(tt.key, tt.value)
			if thread.Metadata.Props[tt.key] != tt.value {
				t.Errorf("expected prop %s to be %s, got %s", tt.key, tt.value, thread.Metadata.Props[tt.key])
			}
		})
	}
}

func TestWithContent(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		content     map[string]any
	}{
		{
			name:        "Update to plain text",
			contentType: "text/plain",
			content:     map[string]any{"text": "Updated content"},
		},
		{
			name:        "Update to JSON",
			contentType: "application/json",
			content:     map[string]any{"json": "Updated JSON content"},
		},
	}

	body := ThreadBody{
		Title:       "Test Thread",
		ContentType: "text/plain",
		Content:     map[string]any{"text": "This is a test thread"},
	}
	author := "@alice:example.com"
	thread := NewThread(body, author)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread.WithContent(tt.contentType, tt.content)
			if thread.Body.ContentType != tt.contentType {
				t.Errorf("expected content type %s, got %s", tt.contentType, thread.Body.ContentType)
			}
			if !compareContent(thread.Body.Content, tt.content) {
				t.Errorf("expected content %v, got %v", tt.content, thread.Body.Content)
			}
		})
	}
}

func TestNewComment(t *testing.T) {
	tests := []struct {
		name     string
		threadID xid.ID
		metadata map[string]string
		body     map[string]any
	}{
		{
			name:     "Basic comment creation",
			threadID: xid.New(),
			metadata: map[string]string{"author": "@bob:example.com"},
			body:     map[string]any{"text": "This is a comment"},
		},
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment, edge := NewComment(tt.threadID, tt.metadata, tt.body)
			if comment.Kind != KindComment {
				t.Errorf("expected kind %s, got %s", KindComment, comment.Kind)
			}
			if comment.Metadata["author"] != tt.metadata["author"] {
				t.Errorf("expected author %s, got %s", tt.metadata["author"], comment.Metadata["author"])
			}
			if comment.Body["text"] != tt.body["text"] {
				t.Errorf("expected body %v, got %v", tt.body, comment.Body)
			}
			if edge.Target != tt.threadID {
				t.Errorf("expected parent ID %s, got %s", tt.threadID, edge.Target)
			}
			if edge.Source != comment.ID {
				t.Errorf("expected child ID %s, got %s", comment.ID, edge.Source)
			}
		})
	}
}
