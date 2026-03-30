package archive

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

// makeEdge builds an Edge with sensible defaults. Source and target must be
// valid node UUIDs that exist in the database.
func makeEdge(repo *core.Repository, source, target *core.Node, edgeType, label string) *core.Edge {
	return &core.Edge{
		ID:        core.NewID(),
		RepoID:    repo.ID,
		Type:      edgeType,
		Source:    source.ID,
		Target:    target.ID,
		Label:     label,
		Weight:    1.0,
		CreatedBy: testOwnerID,
	}
}

// createTwoNodes is a helper that creates two nodes in the given repo.
func createTwoNodes(t *testing.T, a *LocalArchive, repo *core.Repository) (*core.Node, *core.Node) {
	t.Helper()
	nodeA := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "Node A",
		ContentType: "text/plain",
		Body:        json.RawMessage(`"body a"`),
		CreatedBy:   testOwnerID,
	}
	nodeB := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "Node B",
		ContentType: "text/plain",
		Body:        json.RawMessage(`"body b"`),
		CreatedBy:   testOwnerID,
	}
	if err := a.SaveNode(nodeA); err != nil {
		t.Fatalf("SaveNode A failed: %v", err)
	}
	if err := a.SaveNode(nodeB); err != nil {
		t.Fatalf("SaveNode B failed: %v", err)
	}
	return nodeA, nodeB
}

func TestSaveAndGetEdge(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)
	nodeA, nodeB := createTwoNodes(t, a, repo)

	edge := makeEdge(repo, nodeA, nodeB, "reply", "replies to")
	edge.Weight = 0.75

	if err := a.SaveEdge(edge); err != nil {
		t.Fatalf("SaveEdge failed: %v", err)
	}

	if edge.Version != 1 {
		t.Errorf("Expected version 1 after create, got %d", edge.Version)
	}
	if edge.ShortID != "EDGE-1" {
		t.Errorf("Expected short ID EDGE-1, got %s", edge.ShortID)
	}
	if edge.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}

	got, err := a.GetEdge(edge.ID)
	if err != nil {
		t.Fatalf("GetEdge failed: %v", err)
	}

	if got.ID != edge.ID {
		t.Errorf("Expected ID %s, got %s", edge.ID, got.ID)
	}
	if got.RepoID != repo.ID {
		t.Errorf("Expected RepoID %s, got %s", repo.ID, got.RepoID)
	}
	if got.Type != "reply" {
		t.Errorf("Expected type 'reply', got %q", got.Type)
	}
	if got.Source != nodeA.ID {
		t.Errorf("Expected source %s, got %s", nodeA.ID, got.Source)
	}
	if got.Target != nodeB.ID {
		t.Errorf("Expected target %s, got %s", nodeB.ID, got.Target)
	}
	if got.Label != "replies to" {
		t.Errorf("Expected label 'replies to', got %q", got.Label)
	}
	if got.Weight != 0.75 {
		t.Errorf("Expected weight 0.75, got %f", got.Weight)
	}
	if got.CreatedBy != testOwnerID {
		t.Errorf("Expected created_by %s, got %s", testOwnerID, got.CreatedBy)
	}
	if got.Version != 1 {
		t.Errorf("Expected version 1, got %d", got.Version)
	}
}

func TestSaveEdge_Update(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)
	nodeA, nodeB := createTwoNodes(t, a, repo)

	edge := makeEdge(repo, nodeA, nodeB, "reply", "original label")
	if err := a.SaveEdge(edge); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update.
	edge.Label = "updated label"
	edge.Weight = 2.5
	if err := a.SaveEdge(edge); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if edge.Version != 2 {
		t.Errorf("Expected version 2 after update, got %d", edge.Version)
	}

	got, err := a.GetEdge(edge.ID)
	if err != nil {
		t.Fatalf("GetEdge after update failed: %v", err)
	}
	if got.Label != "updated label" {
		t.Errorf("Expected label 'updated label', got %q", got.Label)
	}
	if got.Weight != 2.5 {
		t.Errorf("Expected weight 2.5, got %f", got.Weight)
	}
	if got.Version != 2 {
		t.Errorf("Expected version 2, got %d", got.Version)
	}
}

func TestSaveEdge_VersionConflict(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)
	nodeA, nodeB := createTwoNodes(t, a, repo)

	edge := makeEdge(repo, nodeA, nodeB, "reply", "test")
	if err := a.SaveEdge(edge); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Simulate stale version.
	stale := *edge
	stale.Version = 999
	stale.Label = "Stale Update"

	err := a.SaveEdge(&stale)
	if err == nil {
		t.Fatal("Expected version conflict error, got nil")
	}
	if _, ok := err.(*core.VersionConflictError); !ok {
		t.Errorf("Expected *VersionConflictError, got %T: %v", err, err)
	}
}

func TestSaveBatchEdges(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)
	nodeA, nodeB := createTwoNodes(t, a, repo)

	// Create a third node for variety.
	nodeC := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "Node C",
		ContentType: "text/plain",
		Body:        json.RawMessage(`"body c"`),
		CreatedBy:   testOwnerID,
	}
	if err := a.SaveNode(nodeC); err != nil {
		t.Fatalf("SaveNode C failed: %v", err)
	}

	edges := []*core.Edge{
		makeEdge(repo, nodeA, nodeB, "link", "A to B"),
		makeEdge(repo, nodeB, nodeC, "link", "B to C"),
		makeEdge(repo, nodeA, nodeC, "reply", "A to C"),
	}

	if err := a.SaveBatchEdges(edges); err != nil {
		t.Fatalf("SaveBatchEdges failed: %v", err)
	}

	for i, edge := range edges {
		if edge.Version != 1 {
			t.Errorf("Edge %d: expected version 1, got %d", i, edge.Version)
		}
		expectedShortID := core.FormatShortID(core.PrefixEdge, i+1)
		if edge.ShortID != expectedShortID {
			t.Errorf("Edge %d: expected short ID %s, got %s", i, expectedShortID, edge.ShortID)
		}

		got, err := a.GetEdge(edge.ID)
		if err != nil {
			t.Fatalf("GetEdge for batch edge %d failed: %v", i, err)
		}
		if got.Label != edge.Label {
			t.Errorf("Edge %d: expected label %q, got %q", i, edge.Label, got.Label)
		}
	}
}

func TestDeleteEdge(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)
	nodeA, nodeB := createTwoNodes(t, a, repo)

	edge := makeEdge(repo, nodeA, nodeB, "reply", "to delete")
	if err := a.SaveEdge(edge); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := a.DeleteEdge(edge.ID); err != nil {
		t.Fatalf("DeleteEdge failed: %v", err)
	}

	_, err := a.GetEdge(edge.ID)
	if err == nil {
		t.Error("Expected error after deletion, got nil")
	}
	if _, ok := err.(*core.ErrorNotFound); !ok {
		t.Errorf("Expected *ErrorNotFound, got %T: %v", err, err)
	}
}

func TestListEdges(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)
	nodeA, nodeB := createTwoNodes(t, a, repo)

	nodeC := &core.Node{
		ID:          core.NewID(),
		RepoID:      repo.ID,
		Type:        "note",
		Subject:     "Node C",
		ContentType: "text/plain",
		Body:        json.RawMessage(`"body c"`),
		CreatedBy:   testOwnerID,
	}
	if err := a.SaveNode(nodeC); err != nil {
		t.Fatalf("SaveNode C failed: %v", err)
	}

	// Create edges with different source/target/types.
	edges := []*core.Edge{
		makeEdge(repo, nodeA, nodeB, "reply", "A->B reply"),
		makeEdge(repo, nodeA, nodeC, "link", "A->C link"),
		makeEdge(repo, nodeB, nodeC, "reply", "B->C reply"),
		makeEdge(repo, nodeC, nodeA, "link", "C->A link"),
	}
	for i, e := range edges {
		if err := a.SaveEdge(e); err != nil {
			t.Fatalf("SaveEdge %d failed: %v", i, err)
		}
	}

	// List all edges in repo.
	allEdges, allTotal, err := a.ListEdges(repo.ID, core.EdgeListOptions{ListOptions: core.ListOptions{Limit: 10, Offset: 0}})
	if err != nil {
		t.Fatalf("ListEdges (all) failed: %v", err)
	}
	if allTotal != 4 {
		t.Errorf("Expected 4 total edges, got %d", allTotal)
	}
	if len(allEdges) != 4 {
		t.Errorf("Expected 4 edges, got %d", len(allEdges))
	}

	// Filter by source = nodeA.
	srcEdges, srcTotal, err := a.ListEdges(repo.ID, core.EdgeListOptions{Source: nodeA.ID.String(), ListOptions: core.ListOptions{Limit: 10, Offset: 0}})
	if err != nil {
		t.Fatalf("ListEdges (source filter) failed: %v", err)
	}
	if srcTotal != 2 {
		t.Errorf("Expected 2 edges from nodeA, got %d", srcTotal)
	}
	if len(srcEdges) != 2 {
		t.Errorf("Expected 2 edges from nodeA, got %d", len(srcEdges))
	}

	// Filter by target = nodeC.
	tgtEdges, tgtTotal, err := a.ListEdges(repo.ID, core.EdgeListOptions{Target: nodeC.ID.String(), ListOptions: core.ListOptions{Limit: 10, Offset: 0}})
	if err != nil {
		t.Fatalf("ListEdges (target filter) failed: %v", err)
	}
	if tgtTotal != 2 {
		t.Errorf("Expected 2 edges to nodeC, got %d", tgtTotal)
	}
	if len(tgtEdges) != 2 {
		t.Errorf("Expected 2 edges to nodeC, got %d", len(tgtEdges))
	}

	// Filter by type "reply".
	replyEdges, replyTotal, err := a.ListEdges(repo.ID, core.EdgeListOptions{Type: "reply", ListOptions: core.ListOptions{Limit: 10, Offset: 0}})
	if err != nil {
		t.Fatalf("ListEdges (type filter) failed: %v", err)
	}
	if replyTotal != 2 {
		t.Errorf("Expected 2 reply edges, got %d", replyTotal)
	}
	if len(replyEdges) != 2 {
		t.Errorf("Expected 2 reply edges, got %d", len(replyEdges))
	}

	// Combined filter: source = nodeA, type = "link".
	combined, combinedTotal, err := a.ListEdges(repo.ID, core.EdgeListOptions{Source: nodeA.ID.String(), Type: "link", ListOptions: core.ListOptions{Limit: 10, Offset: 0}})
	if err != nil {
		t.Fatalf("ListEdges (combined filter) failed: %v", err)
	}
	if combinedTotal != 1 {
		t.Errorf("Expected 1 link edge from nodeA, got %d", combinedTotal)
	}
	if len(combined) != 1 {
		t.Errorf("Expected 1 link edge from nodeA, got %d", len(combined))
	}
}
