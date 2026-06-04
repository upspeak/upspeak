package archive

import (
	"encoding/json"
	"testing"

	"github.com/upspeak/upspeak/core"
)

// requireFTS skips the test when the SQLite driver was built without FTS5
// support, mirroring the archive's graceful-degradation behaviour. Run the
// full search suite with `go test -tags sqlite_fts5 ./...`.
func requireFTS(t *testing.T, a *LocalArchive) {
	t.Helper()
	if !a.ftsAvailable {
		t.Skip("FTS5 not available in this build; rebuild with -tags sqlite_fts5")
	}
}

// makeTextNode builds a text/plain node with a controllable subject and body
// so search assertions can target specific terms.
func makeTextNode(repo *core.Repository, nodeType, subject, body string) *core.Node {
	n := makeNode(repo, nodeType, subject)
	b, _ := json.Marshal(body)
	n.Body = b
	return n
}

func TestSearchNodes_FindsSubjectAndBody(t *testing.T) {
	a := setupTestArchive(t)
	requireFTS(t, a)
	repo := createTestRepo(t, a)

	node := makeTextNode(repo, "note", "Quarterly report", "revenue grew sharply")
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}

	results, total, err := a.SearchNodes(repo.ID, "revenue", core.SearchOptions{})
	if err != nil {
		t.Fatalf("SearchNodes failed: %v", err)
	}
	if total != 1 || len(results) != 1 {
		t.Fatalf("expected 1 result with total 1, got %d results total %d", len(results), total)
	}
	if results[0].Node.ID != node.ID {
		t.Errorf("expected node %s, got %s", node.ID, results[0].Node.ID)
	}
	if len(results[0].Highlights) == 0 {
		t.Error("expected at least one highlight")
	}
}

// TestSaveBatchNodes_AreSearchable is a regression test: batch-created nodes
// must be indexed for full-text search just like single saves.
func TestSaveBatchNodes_AreSearchable(t *testing.T) {
	a := setupTestArchive(t)
	requireFTS(t, a)
	repo := createTestRepo(t, a)

	nodes := []*core.Node{
		makeTextNode(repo, "note", "Imported alpha", "first batch body"),
		makeTextNode(repo, "note", "Imported alpha too", "second batch body"),
	}
	if err := a.SaveBatchNodes(nodes); err != nil {
		t.Fatalf("SaveBatchNodes failed: %v", err)
	}

	_, total, err := a.SearchNodes(repo.ID, "alpha", core.SearchOptions{})
	if err != nil {
		t.Fatalf("SearchNodes failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected batch-created nodes to be searchable (total 2), got %d", total)
	}
}

// TestSearchNodes_PaginationTotal verifies the reported total reflects the full
// match set, not the current page, and that pages are bounded by the limit.
func TestSearchNodes_PaginationTotal(t *testing.T) {
	a := setupTestArchive(t)
	requireFTS(t, a)
	repo := createTestRepo(t, a)

	const n = 25
	for i := 0; i < n; i++ {
		node := makeTextNode(repo, "note", "widget item", "shared body text")
		if err := a.SaveNode(node); err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}
	}

	seen := map[string]bool{}
	for offset := 0; offset < n; offset += 10 {
		results, total, err := a.SearchNodes(repo.ID, "widget", core.SearchOptions{Limit: 10, Offset: offset})
		if err != nil {
			t.Fatalf("SearchNodes failed at offset %d: %v", offset, err)
		}
		if total != n {
			t.Errorf("expected total %d at offset %d, got %d", n, offset, total)
		}
		if len(results) > 10 {
			t.Errorf("page exceeded limit: got %d results", len(results))
		}
		for _, r := range results {
			seen[r.Node.ID.String()] = true
		}
	}
	if len(seen) != n {
		t.Errorf("expected to page through %d distinct nodes, saw %d", n, len(seen))
	}
}

// TestSearchNodes_HasEdgeTypeFilter is the key regression for filter + pagination
// correctness: the filter and the total must be computed in SQL, not by dropping
// rows from an already-paginated page.
func TestSearchNodes_HasEdgeTypeFilter(t *testing.T) {
	a := setupTestArchive(t)
	requireFTS(t, a)
	repo := createTestRepo(t, a)

	// 20 matching nodes; only 5 get a "cite" edge to a target node. The target
	// deliberately does NOT match the query term so it is not itself a hit.
	target := makeTextNode(repo, "note", "destination document", "destination body")
	if err := a.SaveNode(target); err != nil {
		t.Fatalf("SaveNode target failed: %v", err)
	}
	var withEdge int
	for i := 0; i < 20; i++ {
		node := makeTextNode(repo, "note", "needle candidate", "candidate body")
		if err := a.SaveNode(node); err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}
		if i < 5 {
			edge := makeEdge(repo, node, target, "cite", "")
			if err := a.SaveEdge(edge); err != nil {
				t.Fatalf("SaveEdge failed: %v", err)
			}
			withEdge++
		}
	}

	results, total, err := a.SearchNodes(repo.ID, "needle", core.SearchOptions{
		HasEdgeType: "cite",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchNodes failed: %v", err)
	}
	if total != withEdge {
		t.Errorf("expected total %d (only edged nodes), got %d", withEdge, total)
	}
	if len(results) != withEdge {
		t.Errorf("expected %d results on the page, got %d", withEdge, len(results))
	}
}

// TestSearchNodes_MetadataFilter verifies metadata key/value filtering happens
// in SQL with a correct total.
func TestSearchNodes_MetadataFilter(t *testing.T) {
	a := setupTestArchive(t)
	requireFTS(t, a)
	repo := createTestRepo(t, a)

	for i := 0; i < 6; i++ {
		node := makeTextNode(repo, "note", "tagged entry", "body")
		status := "open"
		if i%2 == 0 {
			status = "done"
		}
		node.Metadata = []core.Metadata{{Key: "status", Value: json.RawMessage(`"` + status + `"`)}}
		if err := a.SaveNode(node); err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}
	}

	_, total, err := a.SearchNodes(repo.ID, "tagged", core.SearchOptions{
		Metadata: map[string]string{"status": "done"},
	})
	if err != nil {
		t.Fatalf("SearchNodes failed: %v", err)
	}
	if total != 3 {
		t.Errorf("expected 3 nodes with status=done, got %d", total)
	}
}

func TestSearchNodes_MultiTypeFilter(t *testing.T) {
	a := setupTestArchive(t)
	requireFTS(t, a)
	repo := createTestRepo(t, a)

	counts := map[string]int{"note": 4, "task": 3, "event": 5}
	for nodeType, c := range counts {
		for i := 0; i < c; i++ {
			node := makeTextNode(repo, nodeType, "gamma record", "body")
			if err := a.SaveNode(node); err != nil {
				t.Fatalf("SaveNode failed: %v", err)
			}
		}
	}

	results, total, err := a.SearchNodes(repo.ID, "gamma", core.SearchOptions{
		Type:  []string{"note", "task"},
		Limit: 3, // force pagination so a post-filter would corrupt the total
	})
	if err != nil {
		t.Fatalf("SearchNodes failed: %v", err)
	}
	if total != 7 {
		t.Errorf("expected total 7 (note+task), got %d", total)
	}
	for _, r := range results {
		if r.Node.Type != "note" && r.Node.Type != "task" {
			t.Errorf("unexpected node type in results: %q", r.Node.Type)
		}
	}
}

func TestDeleteNode_RemovesFromSearchIndex(t *testing.T) {
	a := setupTestArchive(t)
	requireFTS(t, a)
	repo := createTestRepo(t, a)

	node := makeTextNode(repo, "note", "ephemeral subject", "ephemeral body")
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}
	if err := a.DeleteNode(node.ID); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}

	_, total, err := a.SearchNodes(repo.ID, "ephemeral", core.SearchOptions{})
	if err != nil {
		t.Fatalf("SearchNodes failed: %v", err)
	}
	if total != 0 {
		t.Errorf("expected deleted node to be absent from index, got total %d", total)
	}
}

func TestSearchNodes_ReindexOnUpdate(t *testing.T) {
	a := setupTestArchive(t)
	requireFTS(t, a)
	repo := createTestRepo(t, a)

	node := makeTextNode(repo, "note", "original heading", "original content")
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("SaveNode failed: %v", err)
	}

	node.Subject = "revised heading"
	node.Body = json.RawMessage(`"revised content keyword"`)
	if err := a.SaveNode(node); err != nil {
		t.Fatalf("SaveNode update failed: %v", err)
	}

	if _, total, _ := a.SearchNodes(repo.ID, "original", core.SearchOptions{}); total != 0 {
		t.Errorf("expected old terms to be gone after re-index, got total %d", total)
	}
	if _, total, _ := a.SearchNodes(repo.ID, "keyword", core.SearchOptions{}); total != 1 {
		t.Errorf("expected updated content to be searchable, got total %d", total)
	}
}

func TestSanitiseFTS5Query(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello world", "hello world"},
		{"hyphen not treated as NOT", "foo-bar", "foo bar"},
		{"plus stripped", "c++", "c"},
		{"quotes stripped", `say "hi"`, "say hi"},
		{"boolean operators removed", "foo AND bar OR baz", "foo bar baz"},
		{"special chars", "a*b^c(d)", "a b c d"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitiseFTS5Query(tc.in); got != tc.want {
				t.Errorf("sanitiseFTS5Query(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBrowseNodes(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	for i := 0; i < 5; i++ {
		if err := a.SaveNode(makeNode(repo, "note", "browse subject")); err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := a.SaveNode(makeNode(repo, "task", "browse subject")); err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}
	}

	nodes, total, err := a.BrowseNodes(repo.ID, core.BrowseOptions{Type: "note"})
	if err != nil {
		t.Fatalf("BrowseNodes failed: %v", err)
	}
	if total != 5 {
		t.Errorf("expected 5 note nodes, got total %d", total)
	}
	for _, n := range nodes {
		if n.Type != "note" {
			t.Errorf("expected only note nodes, got %q", n.Type)
		}
	}
}

func TestTraverseGraph(t *testing.T) {
	a := setupTestArchive(t)
	repo := createTestRepo(t, a)

	// Build a small chain: root -> b -> c.
	root := makeNode(repo, "note", "root")
	b := makeNode(repo, "note", "b")
	c := makeNode(repo, "note", "c")
	for _, n := range []*core.Node{root, b, c} {
		if err := a.SaveNode(n); err != nil {
			t.Fatalf("SaveNode failed: %v", err)
		}
	}
	if err := a.SaveEdge(makeEdge(repo, root, b, "link", "")); err != nil {
		t.Fatalf("SaveEdge failed: %v", err)
	}
	if err := a.SaveEdge(makeEdge(repo, b, c, "link", "")); err != nil {
		t.Fatalf("SaveEdge failed: %v", err)
	}

	result, err := a.TraverseGraph(repo.ID, root.ID, 2, core.GraphOptions{Direction: "outgoing"})
	if err != nil {
		t.Fatalf("TraverseGraph failed: %v", err)
	}
	if result.Root == nil || result.Root.ID != root.ID {
		t.Fatalf("expected root %s in result", root.ID)
	}
	if len(result.Nodes) != 2 {
		t.Errorf("expected 2 reachable nodes (b, c) at depth 2, got %d", len(result.Nodes))
	}
}
