package repo

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/api"
	"github.com/upspeak/upspeak/core"
)

// defaultOwnerID is a placeholder until authentication is implemented.
// All repositories are owned by this single user for now.
var defaultOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// createRepoRequest is the expected JSON body for POST /repos.
type createRepoRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// updateRepoRequest is the expected JSON body for PUT /repos/{repo_ref}.
type updateRepoRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// patchRepoRequest is the expected JSON body for PATCH /repos/{repo_ref}.
type patchRepoRequest struct {
	Slug        *string `json:"slug,omitempty"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// createRepoHandler handles POST /api/v1/repos.
func (m *Module) createRepoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createRepoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Slug == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "slug is required")
			return
		}
		if !core.IsValidSlug(req.Slug) {
			api.WriteError(w, http.StatusBadRequest, "invalid_slug", "Slug must be 1-32 lowercase alphanumeric characters or hyphens, starting with alphanumeric")
			return
		}
		if req.Name == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
			return
		}

		// Check slug uniqueness.
		existing, err := m.archive.GetRepositoryBySlug(defaultOwnerID, req.Slug)
		if err == nil && existing != nil {
			api.WriteError(w, http.StatusConflict, "slug_conflict", "A repository with this slug already exists")
			return
		}

		// Check slug is not a redirect.
		_, _, err = m.archive.GetSlugRedirect(defaultOwnerID, req.Slug)
		if err == nil {
			api.WriteError(w, http.StatusConflict, "slug_conflict", "This slug was previously used and cannot be reused")
			return
		}

		repo := &core.Repository{
			ID:          core.NewID(),
			Slug:        req.Slug,
			Name:        req.Name,
			Description: req.Description,
			OwnerID:     defaultOwnerID,
		}

		if err := m.archive.SaveRepository(repo); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to create repository")
			return
		}

		api.SetETag(w, repo.Version)
		api.WriteJSON(w, http.StatusCreated, repo)
	}
}

// listReposHandler handles GET /api/v1/repos.
func (m *Module) listReposHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		opts := api.ParsePagination(r)

		repos, total, err := m.archive.ListRepositories(defaultOwnerID, opts)
		if err != nil {
			api.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list repositories")
			return
		}

		api.WriteList(w, repos, total, opts)
	}
}

// getRepoHandler handles GET /api/v1/repos/{repo_ref}.
func (m *Module) getRepoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("repo_ref")

		repo, err := m.archive.ResolveRepoRef(defaultOwnerID, ref)
		if err != nil {
			var redirectErr *core.ErrorSlugRedirect
			if errors.As(err, &redirectErr) {
				w.Header().Set("Location", "/api/v1/repos/"+redirectErr.NewSlug)
				w.WriteHeader(http.StatusMovedPermanently)
				return
			}

			var notFound *core.ErrorNotFound
			if errors.As(err, &notFound) {
				api.WriteError(w, http.StatusNotFound, "not_found", "Repository not found")
				return
			}

			api.WriteError(w, http.StatusInternalServerError, "resolve_failed", "Failed to resolve repository reference")
			return
		}

		api.SetETag(w, repo.Version)
		api.WriteJSON(w, http.StatusOK, repo)
	}
}

// updateRepoHandler handles PUT /api/v1/repos/{repo_ref}.
func (m *Module) updateRepoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("repo_ref")

		repo, err := m.resolveRepo(w, ref)
		if err != nil {
			return // error already written
		}

		var req updateRepoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if req.Name == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "name is required")
			return
		}
		if req.Slug == "" {
			api.WriteError(w, http.StatusBadRequest, "missing_field", "slug is required")
			return
		}
		if !core.IsValidSlug(req.Slug) {
			api.WriteError(w, http.StatusBadRequest, "invalid_slug", "Invalid slug format")
			return
		}

		// Check If-Match for optimistic concurrency.
		if err := m.checkIfMatch(r, repo, w); err != nil {
			return
		}

		// Handle slug rename.
		if req.Slug != repo.Slug {
			if err := m.handleSlugRename(w, repo, req.Slug); err != nil {
				return
			}
		}

		repo.Slug = req.Slug
		repo.Name = req.Name
		repo.Description = req.Description

		if err := m.archive.SaveRepository(repo); err != nil {
			var conflict *core.VersionConflictError
			if errors.As(err, &conflict) {
				api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified by another request")
				return
			}
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update repository")
			return
		}

		api.SetETag(w, repo.Version)
		api.WriteJSON(w, http.StatusOK, repo)
	}
}

// patchRepoHandler handles PATCH /api/v1/repos/{repo_ref}.
func (m *Module) patchRepoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("repo_ref")

		repo, err := m.resolveRepo(w, ref)
		if err != nil {
			return
		}

		var req patchRepoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			api.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
			return
		}

		if err := m.checkIfMatch(r, repo, w); err != nil {
			return
		}

		// Apply partial updates.
		if req.Name != nil {
			repo.Name = *req.Name
		}
		if req.Description != nil {
			repo.Description = *req.Description
		}
		if req.Slug != nil && *req.Slug != repo.Slug {
			if !core.IsValidSlug(*req.Slug) {
				api.WriteError(w, http.StatusBadRequest, "invalid_slug", "Invalid slug format")
				return
			}
			if err := m.handleSlugRename(w, repo, *req.Slug); err != nil {
				return
			}
			repo.Slug = *req.Slug
		}

		if err := m.archive.SaveRepository(repo); err != nil {
			var conflict *core.VersionConflictError
			if errors.As(err, &conflict) {
				api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified by another request")
				return
			}
			api.WriteError(w, http.StatusInternalServerError, "save_failed", "Failed to update repository")
			return
		}

		api.SetETag(w, repo.Version)
		api.WriteJSON(w, http.StatusOK, repo)
	}
}

// deleteRepoHandler handles DELETE /api/v1/repos/{repo_ref}.
// Returns 202 Accepted (async deletion will be implemented in Phase 3 with jobs).
func (m *Module) deleteRepoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("repo_ref")

		repo, err := m.resolveRepo(w, ref)
		if err != nil {
			return
		}

		// For now, delete synchronously. Phase 3 will make this async via jobs.
		if err := m.archive.DeleteRepository(repo.ID); err != nil {
			api.WriteError(w, http.StatusInternalServerError, "delete_failed", "Failed to delete repository")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// resolveRepo resolves a repo_ref and writes an error response if resolution fails.
// Returns nil, non-nil error if an error was written to the response.
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

// checkIfMatch validates the If-Match header against the repo version.
// Returns non-nil error if a response was already written (412 or 400).
func (m *Module) checkIfMatch(r *http.Request, repo *core.Repository, w http.ResponseWriter) error {
	expected := api.ParseIfMatch(r)
	if expected == -1 {
		api.WriteError(w, http.StatusBadRequest, "invalid_if_match", "Invalid If-Match header value")
		return errors.New("invalid If-Match")
	}
	if expected > 0 && expected != repo.Version {
		api.WriteError(w, http.StatusPreconditionFailed, "version_conflict", "Entity has been modified by another request")
		return errors.New("version conflict")
	}
	return nil
}

// handleSlugRename validates and records a slug rename.
// Returns non-nil error if a response was already written.
func (m *Module) handleSlugRename(w http.ResponseWriter, repo *core.Repository, newSlug string) error {
	// Check new slug is not already in use.
	existing, err := m.archive.GetRepositoryBySlug(defaultOwnerID, newSlug)
	if err == nil && existing != nil {
		api.WriteError(w, http.StatusConflict, "slug_conflict", "A repository with this slug already exists")
		return errors.New("slug conflict")
	}

	// Check new slug is not a redirect.
	_, _, err = m.archive.GetSlugRedirect(defaultOwnerID, newSlug)
	if err == nil {
		api.WriteError(w, http.StatusConflict, "slug_conflict", "This slug was previously used and cannot be reused")
		return errors.New("slug is redirect")
	}

	// Record old slug as redirect.
	if err := m.archive.SaveSlugRedirect(defaultOwnerID, repo.Slug, repo.ID); err != nil {
		api.WriteError(w, http.StatusInternalServerError, "redirect_failed", "Failed to record slug redirect")
		return err
	}

	return nil
}
