// Package search implements the app.Module for full-text search, browsing, and
// graph traversal over a repository's knowledge graph. All search logic lives in
// the archive's SearchStore; this module is a thin HTTP layer that parses
// requests, resolves references, and shapes responses.
package search

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/app"
	"github.com/upspeak/upspeak/core"
)

// defaultOwnerID is a placeholder owner until authentication is implemented.
var defaultOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// maxTraversalDepth bounds graph traversal to keep queries cheap and prevent
// unbounded recursion over densely connected graphs.
const maxTraversalDepth = 5

// Module implements app.Module for search, browse, and graph traversal.
type Module struct {
	archive core.Archive
	logger  *slog.Logger
}

// Name returns the module name.
func (m *Module) Name() string { return "search" }

// Init initialises the search module.
func (m *Module) Init(_ map[string]any) error {
	m.logger = slog.Default().With("module", "search")
	m.logger.Info("Initialised search module")
	return nil
}

// SetArchive injects the archive dependency.
func (m *Module) SetArchive(archive core.Archive) { m.archive = archive }

// HTTPHandlers returns all HTTP route handlers for search. All paths are relative
// to the module's mount point (/api/v1).
func (m *Module) HTTPHandlers() []app.HTTPHandler {
	return []app.HTTPHandler{
		{Method: "POST", Path: "/repos/{repo_ref}/search", Handler: m.searchHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/browse", Handler: m.browseHandler()},
		{Method: "GET", Path: "/repos/{repo_ref}/graph", Handler: m.graphHandler()},
	}
}

// MsgHandlers returns message handlers. None are needed for the search module.
func (m *Module) MsgHandlers() []app.MsgHandler { return []app.MsgHandler{} }

// searchFilters mirrors the inline filters object of a search request body.
type searchFilters struct {
	Type          []string          `json:"type"`
	CreatedAfter  *time.Time        `json:"created_after"`
	CreatedBefore *time.Time        `json:"created_before"`
	HasEdgeType   string            `json:"has_edge_type"`
	Metadata      map[string]string `json:"metadata"`
}

// searchRequest is the JSON body for POST /repos/{repo_ref}/search.
type searchRequest struct {
	Query   string        `json:"query"`
	Filters searchFilters `json:"filters"`
	Limit   int           `json:"limit"`
	Offset  int           `json:"offset"`
}

// searchHandler handles POST /repos/{repo_ref}/search.
func (m *Module) searchHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		r = api.LimitedBody(w, r)
		var req searchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Query == "" {
			api.WriteError(w, http.StatusBadRequest, "validation_failed", "query is required")
			return
		}

		// Surface a clear signal when full-text search is disabled (driver built
		// without FTS5) rather than returning an empty result set as if nothing
		// matched.
		if !m.archive.FTSAvailable() {
			api.WriteError(w, http.StatusServiceUnavailable, "search_unavailable", "Full-text search is disabled on this server")
			return
		}

		opts := core.SearchOptions{
			Type:          req.Filters.Type,
			CreatedAfter:  req.Filters.CreatedAfter,
			CreatedBefore: req.Filters.CreatedBefore,
			HasEdgeType:   req.Filters.HasEdgeType,
			Metadata:      req.Filters.Metadata,
			Limit:         normaliseLimit(req.Limit),
			Offset:        normaliseOffset(req.Offset),
		}

		results, total, err := m.archive.SearchNodes(repo.ID, req.Query, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "search_failed", "Failed to search nodes")
			return
		}

		// Reuse the standard collection envelope so pagination metadata is
		// consistent with other list endpoints.
		api.WriteList(w, results, total, core.ListOptions{Limit: opts.Limit, Offset: opts.Offset})
	}
}

// browseHandler handles GET /repos/{repo_ref}/browse.
func (m *Module) browseHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		opts := core.BrowseOptions{ListOptions: api.ParsePagination(r)}
		if t := r.URL.Query().Get("type"); t != "" {
			opts.Type = t
		}
		if sourceRef := r.URL.Query().Get("source_id"); sourceRef != "" {
			sourceID, err := m.resolveSourceID(repo.ID, sourceRef)
			if err != nil {
				api.WriteError(w, http.StatusBadRequest, "invalid_source_ref", "source_id does not resolve to a source in this repository")
				return
			}
			opts.SourceID = &sourceID
		}

		nodes, total, err := m.archive.BrowseNodes(repo.ID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "browse_failed", "Failed to browse nodes")
			return
		}

		api.WriteList(w, nodes, total, opts.ListOptions)
	}
}

// graphHandler handles GET /repos/{repo_ref}/graph.
func (m *Module) graphHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		repo, err := m.resolveRepo(w, r.PathValue("repo_ref"))
		if err != nil {
			return
		}

		nodeRef := r.URL.Query().Get("node_id")
		if nodeRef == "" {
			api.WriteError(w, http.StatusBadRequest, "validation_failed", "node_id is required")
			return
		}
		startID, err := m.resolveNodeID(repo.ID, nodeRef)
		if err != nil {
			api.WriteError(w, http.StatusNotFound, "not_found", "Start node not found")
			return
		}

		depth := parseDepth(r.URL.Query().Get("depth"))
		opts := core.GraphOptions{
			EdgeType:  r.URL.Query().Get("edge_type"),
			Direction: normaliseDirection(r.URL.Query().Get("direction")),
		}

		result, err := m.archive.TraverseGraph(repo.ID, startID, depth, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "traverse_failed", "Failed to traverse graph")
			return
		}

		api.WriteJSON(w, http.StatusOK, result)
	}
}

// resolveRepo resolves a repository reference and writes HTTP error responses on
// failure.
func (m *Module) resolveRepo(w http.ResponseWriter, ref string) (*core.Repository, error) {
	repo, err := m.archive.ResolveRepoRef(defaultOwnerID, ref)
	if err != nil {
		var redirectErr *core.ErrorSlugRedirect
		if errors.As(err, &redirectErr) {
			w.Header().Set("Location", "/api/v1/repos/"+redirectErr.NewSlug)
			w.WriteHeader(http.StatusMovedPermanently)
			return nil, err
		}
		var notFound *core.ErrorNotFound
		if errors.As(err, &notFound) {
			api.WriteError(w, http.StatusNotFound, "not_found", "Repository not found")
			return nil, err
		}
		api.WriteError(w, http.StatusInternalServerError, "resolve_failed", "Failed to resolve repository reference")
		return nil, err
	}
	return repo, nil
}

// resolveNodeID resolves a node reference (UUID or short ID) to a UUID and
// verifies it belongs to the given repository.
func (m *Module) resolveNodeID(repoID uuid.UUID, ref string) (uuid.UUID, error) {
	id, err := m.resolveEntityID(repoID, ref, "node")
	if err != nil {
		return uuid.Nil, err
	}
	node, err := m.archive.GetNode(id)
	if err != nil || node.RepoID != repoID {
		return uuid.Nil, errors.New("node not found in repository")
	}
	return id, nil
}

// resolveSourceID resolves a source reference (UUID or short ID) to a UUID and
// verifies it belongs to the given repository.
func (m *Module) resolveSourceID(repoID uuid.UUID, ref string) (uuid.UUID, error) {
	id, err := m.resolveEntityID(repoID, ref, "source")
	if err != nil {
		return uuid.Nil, err
	}
	source, err := m.archive.GetSource(id)
	if err != nil || source.RepoID != repoID {
		return uuid.Nil, errors.New("source not found in repository")
	}
	return id, nil
}

// resolveEntityID resolves a UUID or short ID to a canonical UUID, requiring the
// resolved entity to be of the expected type when a short ID is given.
func (m *Module) resolveEntityID(repoID uuid.UUID, ref, expectedType string) (uuid.UUID, error) {
	if id, err := uuid.Parse(ref); err == nil {
		return id, nil
	}
	id, entityType, err := m.archive.ResolveRef(repoID, ref)
	if err != nil {
		return uuid.Nil, err
	}
	if entityType != expectedType {
		return uuid.Nil, errors.New("reference does not resolve to the expected entity type")
	}
	return id, nil
}
