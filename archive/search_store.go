package archive

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// fts5SpecialChars matches FTS5 query syntax operators that could cause
// parse errors or unintended behaviour if passed unsanitised. This includes
// the '-' and '+' operators: a hyphen is FTS5's NOT/exclusion operator, so an
// unsanitised query like "foo-bar" would silently exclude documents containing
// "bar" instead of matching the literal hyphenated term.
var fts5SpecialChars = regexp.MustCompile(`[":*^{}()\[\]+\-]`)

// sanitiseFTS5Query escapes special FTS5 query syntax characters from user
// input to prevent query parse errors. Boolean operators (AND, OR, NOT, NEAR)
// are stripped when they appear as standalone tokens.
func sanitiseFTS5Query(query string) string {
	// Remove FTS5 special characters.
	cleaned := fts5SpecialChars.ReplaceAllString(query, " ")

	// Remove standalone boolean operators (case-sensitive, as FTS5 requires).
	tokens := strings.Fields(cleaned)
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		switch t {
		case "AND", "OR", "NOT", "NEAR":
			continue
		default:
			filtered = append(filtered, t)
		}
	}

	return strings.Join(filtered, " ")
}

// isFTS5SyntaxError checks whether an error is an FTS5 query syntax error.
func isFTS5SyntaxError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "fts5: syntax error") || strings.Contains(msg, "fts5: parse error")
}

// indexNode adds or re-indexes a node in the full-text search table.
// This operation is idempotent — it removes any existing entry before inserting.
func (a *LocalArchive) indexNode(nodeID uuid.UUID, repoID uuid.UUID, subject string, bodyText string) error {
	if !a.ftsAvailable {
		return nil
	}

	tx, err := a.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction for FTS index: %w", err)
	}
	defer tx.Rollback()

	// Remove existing entry to make this idempotent.
	_, err = tx.Exec(`DELETE FROM nodes_fts WHERE node_id = ?`, nodeID.String())
	if err != nil {
		return fmt.Errorf("failed to remove existing FTS entry: %w", err)
	}

	// Insert the new entry.
	_, err = tx.Exec(
		`INSERT INTO nodes_fts(node_id, repo_id, subject, body_text) VALUES (?, ?, ?, ?)`,
		nodeID.String(), repoID.String(), subject, bodyText,
	)
	if err != nil {
		return fmt.Errorf("failed to insert FTS entry: %w", err)
	}

	return tx.Commit()
}

// removeNodeIndex removes a node from the full-text search index.
func (a *LocalArchive) removeNodeIndex(nodeID uuid.UUID) error {
	if !a.ftsAvailable {
		return nil
	}

	_, err := a.db.Exec(`DELETE FROM nodes_fts WHERE node_id = ?`, nodeID.String())
	if err != nil {
		return fmt.Errorf("failed to remove FTS entry: %w", err)
	}
	return nil
}

// searchNodes performs full-text search on nodes within a repository.
// Results are ranked by FTS5 relevance and filtered by optional criteria.
// Returns matching results, total count, and any error.
//
// All filters are applied in SQL so that COUNT(*) and LIMIT/OFFSET operate on
// the same fully-filtered set — pagination and the reported total stay
// consistent regardless of which filters are active.
func (a *LocalArchive) searchNodes(repoID uuid.UUID, query string, opts core.SearchOptions) ([]core.SearchResult, int, error) {
	if !a.ftsAvailable {
		return nil, 0, nil
	}

	// Sanitise user query to prevent FTS5 syntax errors.
	query = sanitiseFTS5Query(query)
	if query == "" {
		return nil, 0, nil
	}

	// Apply sensible defaults for pagination.
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	// Build the WHERE clause and bound args shared by the count and page queries.
	where, args := buildSearchFilter(repoID, query, opts)
	base := `FROM nodes_fts f JOIN nodes n ON f.node_id = n.id ` + where

	// Count total matches.
	var total int
	if err := a.db.QueryRow(`SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		if isFTS5SyntaxError(err) {
			return nil, 0, nil // Invalid query syntax — return empty results.
		}
		return nil, 0, fmt.Errorf("failed to count search results: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	// Fetch the page with highlights.
	resultsQuery := `SELECT f.node_id, f.rank,
highlight(nodes_fts, 2, '<em>', '</em>'),
highlight(nodes_fts, 3, '<em>', '</em>') ` + base + ` ORDER BY f.rank LIMIT ? OFFSET ?`

	resultsArgs := append(append([]any{}, args...), limit, offset)
	rows, err := a.db.Query(resultsQuery, resultsArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to execute search query: %w", err)
	}
	defer rows.Close()

	var results []core.SearchResult
	for rows.Next() {
		var nodeIDStr string
		var rank float64
		var subjectHL, bodyHL string

		if err := rows.Scan(&nodeIDStr, &rank, &subjectHL, &bodyHL); err != nil {
			return nil, 0, fmt.Errorf("failed to scan search result: %w", err)
		}

		nodeID, err := uuid.Parse(nodeIDStr)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse node ID from search result: %w", err)
		}

		// Load the full node.
		node, err := a.getNode(nodeID)
		if err != nil {
			// Node may have been deleted between indexing and search; skip it.
			continue
		}

		// Normalise FTS5 rank to a 0.0–1.0 score.
		// FTS5 rank is negative (closer to 0 is better).
		score := normaliseRank(rank)

		// Build highlights from the highlight strings that actually matched.
		var highlights []core.Highlight
		if strings.Contains(subjectHL, "<em>") {
			highlights = append(highlights, core.Highlight{Field: "subject", Snippet: subjectHL})
		}
		if strings.Contains(bodyHL, "<em>") {
			highlights = append(highlights, core.Highlight{Field: "body", Snippet: bodyHL})
		}

		results = append(results, core.SearchResult{
			Node:       *node,
			Score:      score,
			Highlights: highlights,
		})
	}

	return results, total, nil
}

// buildSearchFilter constructs the WHERE clause and bound arguments shared by
// the search count and result queries. Every filter is expressed in SQL so the
// count and the paginated page operate on the same set.
func buildSearchFilter(repoID uuid.UUID, query string, opts core.SearchOptions) (string, []any) {
	var b strings.Builder
	b.WriteString(`WHERE nodes_fts MATCH ? AND f.repo_id = ?`)
	args := []any{query, repoID.String()}

	// Type filter: n.type IN (...). Handles one or many types uniformly.
	if len(opts.Type) > 0 {
		placeholders := make([]string, len(opts.Type))
		for i, t := range opts.Type {
			placeholders[i] = "?"
			args = append(args, t)
		}
		fmt.Fprintf(&b, ` AND n.type IN (%s)`, strings.Join(placeholders, ", "))
	}

	// Created-at range. Node created_at is stored as RFC3339 UTC, so the same
	// format is used here for a correct lexical comparison.
	if opts.CreatedAfter != nil {
		b.WriteString(` AND n.created_at >= ?`)
		args = append(args, opts.CreatedAfter.UTC().Format(time.RFC3339))
	}
	if opts.CreatedBefore != nil {
		b.WriteString(` AND n.created_at <= ?`)
		args = append(args, opts.CreatedBefore.UTC().Format(time.RFC3339))
	}

	// has_edge_type filter: the node must have at least one edge of the type.
	if opts.HasEdgeType != "" {
		b.WriteString(` AND EXISTS (SELECT 1 FROM edges e WHERE (e.source = n.id OR e.target = n.id) AND e.type = ?)`)
		args = append(args, opts.HasEdgeType)
	}

	// Metadata key/value filters: one EXISTS per pair against the JSON array.
	for k, v := range opts.Metadata {
		b.WriteString(` AND EXISTS (SELECT 1 FROM json_each(n.metadata) je WHERE json_extract(je.value, '$.key') = ? AND json_extract(je.value, '$.value') = ?)`)
		args = append(args, k, v)
	}

	return b.String(), args
}

// browseNodes returns paginated nodes for a repository with optional filters.
// Unlike SearchNodes, this does not perform full-text search.
func (a *LocalArchive) browseNodes(repoID uuid.UUID, opts core.BrowseOptions) ([]core.Node, int, error) {
	where := `WHERE repo_id = ?`
	args := []any{repoID.String()}

	if opts.Type != "" {
		where += ` AND type = ?`
		args = append(args, opts.Type)
	}

	// TODO: SourceID filtering requires a source-to-node tracking mechanism.
	// Nodes do not currently have a source_id field. This will be implemented
	// when source tracking is added to the node model.

	// Count total.
	var total int
	err := a.db.QueryRow(`SELECT COUNT(*) FROM nodes `+where, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count browse results: %w", err)
	}

	// Apply pagination defaults.
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	sortBy := "created_at"
	switch opts.SortBy {
	case "created_at", "updated_at", "short_id", "type", "subject":
		sortBy = opts.SortBy
	}

	order := "DESC"
	if opts.Order == "asc" {
		order = "ASC"
	}

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, type, subject, content_type, metadata, created_by, version, created_at, updated_at
 FROM nodes %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, order,
	)

	queryArgs := append(args, limit, offset)
	rows, err := a.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to browse nodes: %w", err)
	}
	defer rows.Close()

	var nodes []core.Node
	for rows.Next() {
		node, err := scanNodeFromRow(rows)
		if err != nil {
			return nil, 0, err
		}
		nodes = append(nodes, *node)
	}

	return nodes, total, nil
}

// traverseGraph performs a recursive graph traversal from a starting node.
// Depth is capped at 5 to prevent runaway queries.
func (a *LocalArchive) traverseGraph(repoID uuid.UUID, startNodeID uuid.UUID, depth int, opts core.GraphOptions) (*core.GraphResult, error) {
	// Cap depth at 5.
	if depth <= 0 {
		depth = 1
	}
	if depth > 5 {
		depth = 5
	}

	// Verify the start node exists and belongs to this repository.
	rootNode, err := a.getNode(startNodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get root node: %w", err)
	}
	if rootNode.RepoID != repoID {
		return nil, core.NewErrorNotFound("node", startNodeID.String())
	}

	// Build recursive CTE based on direction.
	direction := opts.Direction
	if direction == "" {
		direction = "both"
	}

	var traverseQuery string
	edgeTypeFilter := opts.EdgeType

	switch direction {
	case "outgoing":
		traverseQuery = `
WITH RECURSIVE traverse(node_id, depth) AS (
VALUES(?, 0)
UNION
SELECT e.target, t.depth + 1
FROM edges e
INNER JOIN traverse t ON e.source = t.node_id
WHERE t.depth < ? AND e.repo_id = ?
  AND (? = '' OR e.type = ?)
)
SELECT DISTINCT node_id FROM traverse
`
	case "incoming":
		traverseQuery = `
WITH RECURSIVE traverse(node_id, depth) AS (
VALUES(?, 0)
UNION
SELECT e.source, t.depth + 1
FROM edges e
INNER JOIN traverse t ON e.target = t.node_id
WHERE t.depth < ? AND e.repo_id = ?
  AND (? = '' OR e.type = ?)
)
SELECT DISTINCT node_id FROM traverse
`
	default: // "both"
		// Use iterative approach to prevent combinatorial explosion in cyclic graphs.
		return a.traverseGraphBoth(repoID, startNodeID, rootNode, depth, edgeTypeFilter)
	}

	traverseArgs := []any{startNodeID.String(), depth, repoID.String(), edgeTypeFilter, edgeTypeFilter}
	rows, err := a.db.Query(traverseQuery, traverseArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute graph traversal: %w", err)
	}
	defer rows.Close()

	// Collect all traversed node IDs.
	nodeIDSet := make(map[uuid.UUID]bool)
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, fmt.Errorf("failed to scan traversal result: %w", err)
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse traversal node ID: %w", err)
		}
		nodeIDSet[id] = true
	}

	// Batch-load all nodes (excluding root, which we already have).
	var nodes []core.Node
	for id := range nodeIDSet {
		if id == startNodeID {
			continue
		}
		node, err := a.getNode(id)
		if err != nil {
			// Node may have been deleted; skip it.
			continue
		}
		nodes = append(nodes, *node)
	}

	// Collect edges between traversed nodes.
	edges, err := a.getEdgesBetweenNodes(repoID, nodeIDSet, edgeTypeFilter)
	if err != nil {
		return nil, fmt.Errorf("failed to get edges between traversed nodes: %w", err)
	}

	return &core.GraphResult{
		Root:  rootNode,
		Nodes: nodes,
		Edges: edges,
	}, nil
}

// getEdgesBetweenNodes retrieves all edges where both source and target are
// in the given node set. Optionally filters by edge type.
func (a *LocalArchive) getEdgesBetweenNodes(repoID uuid.UUID, nodeIDs map[uuid.UUID]bool, edgeType string) ([]core.Edge, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}

	// Build a set of placeholders for the IN clause.
	placeholders := make([]string, 0, len(nodeIDs))
	args := make([]any, 0, len(nodeIDs)*2+2)

	// We need source IN (...) AND target IN (...).
	for id := range nodeIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id.String())
	}

	inClause := strings.Join(placeholders, ", ")

	// Duplicate the placeholders for both source and target.
	targetArgs := make([]any, len(args))
	copy(targetArgs, args)

	query := fmt.Sprintf(
		`SELECT id, short_id, repo_id, type, source, target, label, weight, created_by, version, created_at, updated_at
 FROM edges
 WHERE repo_id = ? AND source IN (%s) AND target IN (%s)`,
		inClause, inClause,
	)

	// Build final args: repo_id, source IDs, target IDs.
	finalArgs := []any{repoID.String()}
	finalArgs = append(finalArgs, args...)
	finalArgs = append(finalArgs, targetArgs...)

	if edgeType != "" {
		query += ` AND type = ?`
		finalArgs = append(finalArgs, edgeType)
	}

	rows, err := a.db.Query(query, finalArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query edges between nodes: %w", err)
	}
	defer rows.Close()

	var edges []core.Edge
	for rows.Next() {
		edge, err := scanEdgeFromRow(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, *edge)
	}

	return edges, nil
}

// normaliseRank converts FTS5 rank (negative, closer to 0 = better) to a
// 0.0–1.0 score where 1.0 is the best match.
func normaliseRank(rank float64) float64 {
	if rank >= 0 {
		return 1.0
	}
	// Use exponential decay: score = e^rank (rank is negative).
	score := math.Exp(rank)
	if score > 1.0 {
		return 1.0
	}
	if score < 0.0 {
		return 0.0
	}
	return score
}

// traverseGraphBoth performs iterative BFS in both directions, maintaining a
// visited set to prevent combinatorial explosion in cyclic graphs.
func (a *LocalArchive) traverseGraphBoth(repoID uuid.UUID, startNodeID uuid.UUID, rootNode *core.Node, maxDepth int, edgeType string) (*core.GraphResult, error) {
	visited := make(map[uuid.UUID]bool)
	visited[startNodeID] = true
	frontier := []uuid.UUID{startNodeID}

	for d := 0; d < maxDepth && len(frontier) > 0; d++ {
		// Build placeholders for current frontier.
		placeholders := make([]string, len(frontier))
		args := make([]any, 0, len(frontier)+2)
		args = append(args, repoID.String())
		for i, id := range frontier {
			placeholders[i] = "?"
			args = append(args, id.String())
		}
		inClause := strings.Join(placeholders, ", ")

		query := fmt.Sprintf(
			`SELECT source, target FROM edges WHERE repo_id = ? AND (source IN (%s) OR target IN (%s))`,
			inClause, inClause,
		)

		// Duplicate frontier args for both IN clauses.
		frontierArgs := args[1:] // skip repo_id
		finalArgs := []any{repoID.String()}
		finalArgs = append(finalArgs, frontierArgs...)
		finalArgs = append(finalArgs, frontierArgs...)

		if edgeType != "" {
			query += ` AND type = ?`
			finalArgs = append(finalArgs, edgeType)
		}

		rows, err := a.db.Query(query, finalArgs...)
		if err != nil {
			return nil, fmt.Errorf("failed to query edges in BFS: %w", err)
		}

		var nextFrontier []uuid.UUID
		for rows.Next() {
			var srcStr, tgtStr string
			if err := rows.Scan(&srcStr, &tgtStr); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan edge in BFS: %w", err)
			}
			src, _ := uuid.Parse(srcStr)
			tgt, _ := uuid.Parse(tgtStr)

			if !visited[src] {
				visited[src] = true
				nextFrontier = append(nextFrontier, src)
			}
			if !visited[tgt] {
				visited[tgt] = true
				nextFrontier = append(nextFrontier, tgt)
			}
		}
		rows.Close()
		frontier = nextFrontier
	}

	// Load all visited nodes (excluding root).
	var nodes []core.Node
	for id := range visited {
		if id == startNodeID {
			continue
		}
		node, err := a.getNode(id)
		if err != nil {
			continue // Deleted between traversal and load.
		}
		nodes = append(nodes, *node)
	}

	// Collect edges between visited nodes.
	edges, err := a.getEdgesBetweenNodes(repoID, visited, edgeType)
	if err != nil {
		return nil, fmt.Errorf("failed to get edges between traversed nodes: %w", err)
	}

	return &core.GraphResult{
		Root:  rootNode,
		Nodes: nodes,
		Edges: edges,
	}, nil
}
