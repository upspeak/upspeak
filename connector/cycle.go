package connector

import (
	"github.com/google/uuid"

	"github.com/upspeak/upspeak/core"
)

// detectCycle performs a BFS from targetRepoID through repo-type sources and
// sinks to determine if adding a connection from startRepoID to targetRepoID
// would create a circular reference. Returns true if a cycle would be formed.
func (m *Module) detectCycle(startRepoID, targetRepoID uuid.UUID) (bool, error) {
	if startRepoID == targetRepoID {
		return true, nil
	}

	visited := map[uuid.UUID]bool{}
	queue := []uuid.UUID{targetRepoID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == startRepoID {
			return true, nil
		}
		if visited[current] {
			continue
		}
		visited[current] = true

		repoIDs, err := m.repoConnectorTargets(current)
		if err != nil {
			return false, err
		}
		queue = append(queue, repoIDs...)
	}

	return false, nil
}

// repoConnectorTargets returns all repo IDs referenced by repo-type sources
// and sinks in the given repository.
func (m *Module) repoConnectorTargets(repoID uuid.UUID) ([]uuid.UUID, error) {
	var targets []uuid.UUID

	// Check sources. Use a high limit to ensure all repo-type connectors are found.
	sources, _, err := m.archive.ListSources(repoID, core.SourceListOptions{
		Connector:   core.ConnectorRepo,
		ListOptions: core.ListOptions{Limit: 1000, SortBy: "created_at", Order: "desc"},
	})
	if err != nil {
		return nil, err
	}
	for _, src := range sources {
		if rid := extractRepoID(src.Config); rid != uuid.Nil {
			targets = append(targets, rid)
		}
	}

	// Check sinks.
	sinks, _, err := m.archive.ListSinks(repoID, core.SinkListOptions{
		Connector:   core.ConnectorRepo,
		ListOptions: core.ListOptions{Limit: 1000, SortBy: "created_at", Order: "desc"},
	})
	if err != nil {
		return nil, err
	}
	for _, sink := range sinks {
		if rid := extractRepoID(sink.Config); rid != uuid.Nil {
			targets = append(targets, rid)
		}
	}

	return targets, nil
}

// extractRepoID extracts and parses the repo_id field from a connector config map.
func extractRepoID(config map[string]any) uuid.UUID {
	repoIDStr, ok := config["repo_id"].(string)
	if !ok {
		return uuid.Nil
	}
	rid, err := uuid.Parse(repoIDStr)
	if err != nil {
		return uuid.Nil
	}
	return rid
}
