package archive

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/upspeak/upspeak/core"
)

// resolveRef resolves a short ID (e.g. "NODE-42") or UUID string to the
// canonical UUID and entity type within a repository.
//
// Resolution order:
//  1. Try as UUID: search nodes, edges, threads, annotations tables in order.
//  2. Try as short ID: parse prefix to determine entity type, query the
//     appropriate table by repo_id and short_id.
//  3. Return ErrorNotFound if nothing matches.
func (a *LocalArchive) resolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error) {
	// 1. Try as UUID.
	if id, err := uuid.Parse(ref); err == nil {
		return a.resolveByUUID(repoID, id)
	}

	// 2. Try as short ID.
	if core.IsShortID(ref) {
		return a.resolveByShortID(repoID, ref)
	}

	return uuid.Nil, "", core.NewErrorNotFound("entity", ref)
}

// resolveByUUID searches entity tables in order for a matching UUID within
// the given repository.
func (a *LocalArchive) resolveByUUID(repoID uuid.UUID, id uuid.UUID) (uuid.UUID, string, error) {
	tables := []struct {
		table      string
		entityType string
	}{
		{"nodes", "node"},
		{"edges", "edge"},
		{"threads", "thread"},
		{"annotations", "annotation"},
	}

	for _, t := range tables {
		var exists int
		err := a.db.QueryRow(
			fmt.Sprintf(`SELECT 1 FROM %s WHERE id = ? AND repo_id = ? LIMIT 1`, t.table),
			id.String(), repoID.String(),
		).Scan(&exists)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return uuid.Nil, "", fmt.Errorf("failed to check %s table: %w", t.table, err)
		}
		return id, t.entityType, nil
	}

	return uuid.Nil, "", core.NewErrorNotFound("entity", id.String())
}

// resolveByShortID parses the short ID prefix to determine entity type
// and queries the appropriate table.
func (a *LocalArchive) resolveByShortID(repoID uuid.UUID, ref string) (uuid.UUID, string, error) {
	prefix, _, err := core.ParseShortID(ref)
	if err != nil {
		return uuid.Nil, "", core.NewErrorNotFound("entity", ref)
	}

	entityType, ok := core.EntityPrefixToType[prefix]
	if !ok {
		return uuid.Nil, "", core.NewErrorNotFound("entity", ref)
	}

	table, err := entityTypeToTable(entityType)
	if err != nil {
		return uuid.Nil, "", err
	}

	var idStr string
	err = a.db.QueryRow(
		fmt.Sprintf(`SELECT id FROM %s WHERE repo_id = ? AND short_id = ? LIMIT 1`, table),
		repoID.String(), ref,
	).Scan(&idStr)
	if err == sql.ErrNoRows {
		return uuid.Nil, "", core.NewErrorNotFound(entityType, ref)
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to resolve short ID %s: %w", ref, err)
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("failed to parse resolved ID: %w", err)
	}

	return id, entityType, nil
}

// entityTypeToTable maps an entity type name to its database table name.
func entityTypeToTable(entityType string) (string, error) {
	switch entityType {
	case "node":
		return "nodes", nil
	case "edge":
		return "edges", nil
	case "thread":
		return "threads", nil
	case "annotation":
		return "annotations", nil
	default:
		return "", fmt.Errorf("unsupported entity type for resolution: %s", entityType)
	}
}
