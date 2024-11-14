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

func validateThread(t *testing.T, thread Thread, expectedAuthor, expectedTitle, expectedContentType string, expectedContent map[string]any) {
	if thread.Metadata.Author != expectedAuthor {
		t.Errorf("expected author %s, got %s", expectedAuthor, thread.Metadata.Author)
	}
	if thread.Metadata.Version != 1 {
		t.Errorf("expected version 1, got %d", thread.Metadata.Version)
	}
	if thread.Body.Title != expectedTitle {
		t.Errorf("expected title %s, got %s", expectedTitle, thread.Body.Title)
	}
	if thread.Body.ContentType != expectedContentType {
		t.Errorf("expected content type %s, got %s", expectedContentType, thread.Body.ContentType)
	}
	if !compareContent(thread.Body.Content, expectedContent) {
		t.Errorf("expected content %v, got %v", expectedContent, thread.Body.Content)
	}
	if thread.Kind != KindThread {
		t.Errorf("expected kind %s, got %s", KindThread, thread.Kind)
	}
}

func validateComment(t *testing.T, comment Node[map[string]string, map[string]any], expectedKind Kind, expectedAuthor string, expectedBody map[string]any, edge Edge, expectedParentID xid.ID) {
	if comment.Kind != expectedKind {
		t.Errorf("expected kind %s, got %s", expectedKind, comment.Kind)
	}
	if comment.Metadata["author"] != expectedAuthor {
		t.Errorf("expected author %s, got %s", expectedAuthor, comment.Metadata["author"])
	}
	if !compareContent(comment.Body, expectedBody) {
		t.Errorf("expected body %v, got %v", expectedBody, comment.Body)
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
			validateThread(t, thread, tt.author, tt.body.Title, tt.body.ContentType, tt.body.Content)
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
			validateComment(t, comment, KindComment, tt.metadata["author"], tt.body, edge, tt.threadID)
		})
	}
}

func TestThreadWithMultipleCommentsWithReply(t *testing.T) {
	threadBody := ThreadBody{
		Title:       "Test Thread",
		ContentType: "text/plain",
		Content:     map[string]any{"text": "This is my simple thread's simple description"},
	}
	author := "@alice:example.com"
	thread := NewThread(threadBody, author)

	// First comment
	firstCommentBody := map[string]any{"text": "This is the first comment"}
	firstCommentMetadata := map[string]string{"author": "@bob:example.com"}
	firstComment, firstEdge := NewComment(thread.ID, firstCommentMetadata, firstCommentBody)

	// Second comment
	secondCommentBody := map[string]any{"text": "This is the second comment"}
	secondCommentMetadata := map[string]string{"author": "@carol:example.com"}
	secondComment, secondEdge := NewComment(thread.ID, secondCommentMetadata, secondCommentBody)

	// Third comment (Image Node) in reply to the second comment
	thirdCommentBody := map[string]any{"image_url": "http://example.com/image.png"}
	thirdCommentMetadata := map[string]string{"author": "@dave:example.com"}
	thirdComment, thirdEdge := NewComment(thread.ID, thirdCommentMetadata, thirdCommentBody)
	thirdCommentReplyEdge := ReplyEdge(thirdComment.ID, secondComment.ID)

	// Validate the thread
	validateThread(t, thread, author, threadBody.Title, threadBody.ContentType, threadBody.Content)

	// Validate first comment
	validateComment(t, firstComment, KindComment, firstCommentMetadata["author"], firstCommentBody, firstEdge, thread.ID)

	// Validate second comment
	validateComment(t, secondComment, KindComment, secondCommentMetadata["author"], secondCommentBody, secondEdge, thread.ID)

	// Validate third comment
	validateComment(t, thirdComment, KindComment, thirdCommentMetadata["author"], thirdCommentBody, thirdEdge, thread.ID)

	// Validate third comment's reply edge
	validateEdge(t, thirdCommentReplyEdge, "Reply", thirdComment.ID, secondComment.ID, 0.5, "")
}
