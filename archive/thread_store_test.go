package archive

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

// makeThread builds a Thread with a root node and no initial edges.
func makeThread(repo *core.Repository, subject string) *core.Thread {
	return &core.Thread{
		ID:     core.NewID(),
		RepoID: repo.ID,
		Node: core.Node{
			ID:          core.NewID(),
			RepoID:      repo.ID,
			Type:        "thread-root",
			Subject:     subject,
			ContentType: "text/plain",
			Body:        json.RawMessage(`"thread root body"`),
			CreatedBy:   testOwnerID,
		},
		Metadata:  []core.Metadata{{Key: "topic", Value: json.RawMessage(`"general"`)}},
		CreatedBy: testOwnerID,
	}
}

func TestSaveAndGetThread(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	thread := makeThread(repo, "Discussion Thread")

	if err := a.SaveThread(thread); err != nil {
		t.Fatalf("SaveThread failed: %v", err)
	}

	if thread.Version != 1 {
		t.Errorf("Expected version 1 after create, got %d", thread.Version)
	}
	if thread.ShortID != "THREAD-1" {
		t.Errorf("Expected short ID THREAD-1, got %s", thread.ShortID)
	}
	if thread.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
	// The root node should also have been saved with its own short ID.
	if thread.Node.ShortID == "" {
		t.Error("Expected root node to have a short ID")
	}
	if thread.Node.Version != 1 {
		t.Errorf("Expected root node version 1, got %d", thread.Node.Version)
	}

	got, err := a.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread failed: %v", err)
	}

	if got.ID != thread.ID {
		t.Errorf("Expected ID %s, got %s", thread.ID, got.ID)
	}
	if got.RepoID != repo.ID {
		t.Errorf("Expected RepoID %s, got %s", repo.ID, got.RepoID)
	}
	if got.ShortID != "THREAD-1" {
		t.Errorf("Expected short ID THREAD-1, got %s", got.ShortID)
	}
	if got.Node.ID != thread.Node.ID {
		t.Errorf("Expected root node ID %s, got %s", thread.Node.ID, got.Node.ID)
	}
	if got.Node.Subject != "Discussion Thread" {
		t.Errorf("Expected root node subject 'Discussion Thread', got %q", got.Node.Subject)
	}
	if got.CreatedBy != testOwnerID {
		t.Errorf("Expected created_by %s, got %s", testOwnerID, got.CreatedBy)
	}
	if len(got.Metadata) != 1 || got.Metadata[0].Key != "topic" {
		t.Errorf("Expected metadata with key 'topic', got %+v", got.Metadata)
	}
	if got.Version != 1 {
		t.Errorf("Expected version 1, got %d", got.Version)
	}
}

func TestSaveThread_Update(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	thread := makeThread(repo, "Original Thread")
	if err := a.SaveThread(thread); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update thread metadata.
	thread.Metadata = []core.Metadata{
		{Key: "topic", Value: json.RawMessage(`"updated topic"`)},
		{Key: "priority", Value: json.RawMessage(`"high"`)},
	}
	if err := a.SaveThread(thread); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if thread.Version != 2 {
		t.Errorf("Expected version 2 after update, got %d", thread.Version)
	}

	got, err := a.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread after update failed: %v", err)
	}
	if len(got.Metadata) != 2 {
		t.Errorf("Expected 2 metadata entries, got %d", len(got.Metadata))
	}
	if got.Version != 2 {
		t.Errorf("Expected version 2, got %d", got.Version)
	}
}

func TestDeleteThread(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	thread := makeThread(repo, "Thread to Delete")
	if err := a.SaveThread(thread); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Add an extra node to the thread.
	extraNode := makeNode(repo, "note", "Extra Node")
	if err := a.SaveNode(extraNode); err != nil {
		t.Fatalf("SaveNode (extra) failed: %v", err)
	}
	if err := a.AddNodeToThread(thread.ID, extraNode.ID, "reply"); err != nil {
		t.Fatalf("AddNodeToThread failed: %v", err)
	}

	rootNodeID := thread.Node.ID

	// Delete the thread.
	if err := a.DeleteThread(thread.ID); err != nil {
		t.Fatalf("DeleteThread failed: %v", err)
	}

	// Thread should be gone.
	_, err := a.GetThread(thread.ID)
	if err == nil {
		t.Error("Expected error after thread deletion, got nil")
	}

	// Root node should be deleted.
	_, err = a.GetNode(rootNodeID)
	if err == nil {
		t.Error("Expected root node to be deleted")
	}

	// Extra node should still exist (only the root node is deleted).
	got, err := a.GetNode(extraNode.ID)
	if err != nil {
		t.Fatalf("Extra node should still exist: %v", err)
	}
	if got.ID != extraNode.ID {
		t.Errorf("Expected extra node ID %s, got %s", extraNode.ID, got.ID)
	}
}

func TestListThreads(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	for i := 0; i < 3; i++ {
		thread := makeThread(repo, "Thread")
		// Give each thread a unique root node ID.
		thread.ID = core.NewID()
		thread.Node.ID = core.NewID()
		if err := a.SaveThread(thread); err != nil {
			t.Fatalf("SaveThread %d failed: %v", i, err)
		}
	}

	// List all threads.
	threads, total, err := a.ListThreads(repo.ID, core.ListOptions{Limit: 10, Offset: 0, SortBy: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("ListThreads failed: %v", err)
	}
	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
	if len(threads) != 3 {
		t.Errorf("Expected 3 threads, got %d", len(threads))
	}

	// Test pagination: limit 2, offset 0.
	page1, pagTotal, err := a.ListThreads(repo.ID, core.ListOptions{Limit: 2, Offset: 0, SortBy: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("ListThreads (page 1) failed: %v", err)
	}
	if pagTotal != 3 {
		t.Errorf("Expected total 3 with pagination, got %d", pagTotal)
	}
	if len(page1) != 2 {
		t.Errorf("Expected 2 threads in page 1, got %d", len(page1))
	}

	// Pagination: limit 2, offset 2 (should get 1 result).
	page2, _, err := a.ListThreads(repo.ID, core.ListOptions{Limit: 2, Offset: 2, SortBy: "created_at", Order: "asc"})
	if err != nil {
		t.Fatalf("ListThreads (page 2) failed: %v", err)
	}
	if len(page2) != 1 {
		t.Errorf("Expected 1 thread in last page, got %d", len(page2))
	}
}

func TestAddNodeToThread(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	thread := makeThread(repo, "Thread with Additions")
	if err := a.SaveThread(thread); err != nil {
		t.Fatalf("SaveThread failed: %v", err)
	}

	// Create a separate node.
	extraNode := makeNode(repo, "note", "Extra Reply")
	if err := a.SaveNode(extraNode); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}

	// Add the node to the thread.
	if err := a.AddNodeToThread(thread.ID, extraNode.ID, "reply"); err != nil {
		t.Fatalf("AddNodeToThread failed: %v", err)
	}

	// Verify via GetThread.
	got, err := a.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread failed: %v", err)
	}

	if len(got.Edges) != 1 {
		t.Fatalf("Expected 1 edge after AddNodeToThread, got %d", len(got.Edges))
	}

	edge := got.Edges[0]
	if edge.Source != thread.Node.ID {
		t.Errorf("Expected edge source to be root node %s, got %s", thread.Node.ID, edge.Source)
	}
	if edge.Target != extraNode.ID {
		t.Errorf("Expected edge target to be extra node %s, got %s", extraNode.ID, edge.Target)
	}
	if edge.Type != "reply" {
		t.Errorf("Expected edge type 'reply', got %q", edge.Type)
	}
}

func TestRemoveNodeFromThread(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	thread := makeThread(repo, "Thread with Removal")
	if err := a.SaveThread(thread); err != nil {
		t.Fatalf("SaveThread failed: %v", err)
	}

	// Create and add a node.
	extraNode := makeNode(repo, "note", "To Remove")
	if err := a.SaveNode(extraNode); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}
	if err := a.AddNodeToThread(thread.ID, extraNode.ID, "reply"); err != nil {
		t.Fatalf("AddNodeToThread failed: %v", err)
	}

	// Verify edge exists.
	gotBefore, err := a.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread (before removal) failed: %v", err)
	}
	if len(gotBefore.Edges) != 1 {
		t.Fatalf("Expected 1 edge before removal, got %d", len(gotBefore.Edges))
	}

	// Remove the node from the thread.
	if err := a.RemoveNodeFromThread(thread.ID, extraNode.ID); err != nil {
		t.Fatalf("RemoveNodeFromThread failed: %v", err)
	}

	// Verify the edge is gone.
	gotAfter, err := a.GetThread(thread.ID)
	if err != nil {
		t.Fatalf("GetThread (after removal) failed: %v", err)
	}
	if len(gotAfter.Edges) != 0 {
		t.Errorf("Expected 0 edges after removal, got %d", len(gotAfter.Edges))
	}

	// The extra node itself should still exist.
	_, err = a.GetNode(extraNode.ID)
	if err != nil {
		t.Errorf("Extra node should still exist after removal from thread: %v", err)
	}
}
