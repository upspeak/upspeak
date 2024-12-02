package core

import (
	"encoding/json"
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

func validateThread(t *testing.T, thread Node, expectedAuthor, expectedTitle, expectedContentType string) {
	var metadata ThreadMetadata
	var body ThreadBody

	if err := json.Unmarshal(thread.Metadata, &metadata); err != nil {
		t.Errorf("failed to unmarshal metadata: %v", err)
	}
	if err := json.Unmarshal(thread.Body, &body); err != nil {
		t.Errorf("failed to unmarshal body: %v", err)
	}

	if thread.ContentType != expectedContentType {
		t.Errorf("expected content type %s, got %s", expectedContentType, thread.ContentType)
	}

	if metadata.Author != expectedAuthor {
		t.Errorf("expected author %s, got %s", expectedAuthor, metadata.Author)
	}
	if metadata.Version != 1 {
		t.Errorf("expected version 1, got %d", metadata.Version)
	}
	if body.Title != expectedTitle {
		t.Errorf("expected title %s, got %s", expectedTitle, body.Title)
	}

	if thread.Kind != KindThread {
		t.Errorf("expected kind %s, got %s", KindThread, thread.Kind)
	}
}

func validateComment(t *testing.T, comment Node, expectedKind Kind, edge Edge, expectedParentID xid.ID) {
	if comment.Kind != expectedKind {
		t.Errorf("expected kind %s, got %s", expectedKind, comment.Kind)
	}
	if edge.Target != expectedParentID {
		t.Errorf("expected parent ID %s, got %s", expectedParentID, edge.Target)
	}
	if edge.Source != comment.ID {
		t.Errorf("expected child ID %s, got %s", comment.ID, edge.Source)
	}
}

func TestNewThread(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        ThreadBody
		author      string
	}{
		{
			name:        "Basic thread creation",
			contentType: "application/json",
			body: ThreadBody{
				Title:   "Test Thread",
				Content: json.RawMessage(`{"text": "This is a test thread"}`),
			},
			author: "@alice:example.com",
		},
		{
			name:        "Basic thread creation with nil content",
			contentType: "text/plain",
			body: ThreadBody{
				Title:   "Test Thread with no added content",
				Content: nil,
			},
			author: "@bob:example.com",
		},
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread, err := NewThread(tt.contentType, tt.body, tt.author)
			if err != nil {
				t.Fatalf("failed to create thread: %v", err)
			}
			validateThread(t, thread, tt.author, tt.body.Title, tt.contentType)
		})
	}
}

func TestNewComment(t *testing.T) {
	tests := []struct {
		name        string
		threadID    xid.ID
		contentType string
		metadata    map[string]string
		body        map[string]any
	}{
		{
			name:        "Basic comment creation",
			threadID:    xid.New(),
			contentType: "application/json",
			metadata:    map[string]string{"author": "@bob:example.com"},
			body:        map[string]any{"text": "This is a comment"},
		},
		// Add more test cases here
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := json.Marshal(tt.metadata)
			if err != nil {
				t.Fatalf("failed to marshal metadata: %v", err)
			}
			body, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("failed to marshal body: %v", err)
			}
			comment, edge := NewComment(tt.threadID, tt.contentType, json.RawMessage(metadata), json.RawMessage(body))
			validateComment(t, comment, KindComment, edge, tt.threadID)
		})
	}
}

func TestThreadWithMultipleCommentsWithReply(t *testing.T) {
	threadBody := ThreadBody{
		Title:   "Test Thread",
		Content: json.RawMessage(`{"text": "This is my simple thread's simple description"}`),
	}
	author := "@alice:example.com"
	thread, err := NewThread("text/plain", threadBody, author)
	if err != nil {
		t.Fatalf("failed to create thread: %v", err)
	}

	// First comment
	firstCommentBody := map[string]any{"text": "This is the first comment"}
	firstCommentMetadata := map[string]string{"author": "@bob:example.com"}
	firstCommentMetadataJSON, _ := json.Marshal(firstCommentMetadata)
	firstCommentBodyJSON, _ := json.Marshal(firstCommentBody)
	firstComment, firstEdge := NewComment(thread.ID, "application/json", json.RawMessage(firstCommentMetadataJSON), json.RawMessage(firstCommentBodyJSON))

	// Second comment
	secondCommentBody := map[string]any{"text": "This is the second comment"}
	secondCommentMetadata := map[string]string{"author": "@carol:example.com"}
	secondCommentMetadataJSON, _ := json.Marshal(secondCommentMetadata)
	secondCommentBodyJSON, _ := json.Marshal(secondCommentBody)
	secondComment, secondEdge := NewComment(thread.ID, "application/json", json.RawMessage(secondCommentMetadataJSON), json.RawMessage(secondCommentBodyJSON))

	// Third comment (Image Node) in reply to the second comment
	thirdCommentBody := map[string]any{"image_url": "http://example.com/image.png"}
	thirdCommentMetadata := map[string]string{"author": "@dave:example.com"}
	thirdCommentMetadataJSON, _ := json.Marshal(thirdCommentMetadata)
	thirdCommentBodyJSON, _ := json.Marshal(thirdCommentBody)
	thirdComment, thirdEdge := NewComment(thread.ID, "application/json", json.RawMessage(thirdCommentMetadataJSON), json.RawMessage(thirdCommentBodyJSON))
	thirdCommentReplyEdge := ReplyEdge(thirdComment.ID, secondComment.ID)

	// Validate the thread
	validateThread(t, thread, author, threadBody.Title, "text/plain")

	// Validate first comment
	validateComment(t, firstComment, KindComment, firstEdge, thread.ID)

	// Validate second comment
	validateComment(t, secondComment, KindComment, secondEdge, thread.ID)

	// Validate third comment
	validateComment(t, thirdComment, KindComment, thirdEdge, thread.ID)

	// Validate third comment's reply edge
	validateEdge(t, thirdCommentReplyEdge, "Reply", thirdComment.ID, secondComment.ID, 0.5, "")
}
