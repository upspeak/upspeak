package connector

import (
	"github.com/google/uuid"

	"github.com/upspeak/upspeak/core"
)

// detectCycle performs a BFS from targetRepoID through repo-type Sources to
// determine if adding a connection from startRepoID to targetRepoID would create
// a circular reference. Returns true if a cycle would be formed.
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

// repoConnectorTargets returns the repo IDs that repo-type Sources in the given
// repository subscribe to, resolved via each Source's referenced Sink. A repo
// Source declares config.sink_id (a Sink in another repository); data flows from
// that Sink's repository into this one, so the Sink's owner repo is the connector
// target. Sinks themselves are target-agnostic and contribute no edges.
func (m *Module) repoConnectorTargets(repoID uuid.UUID) ([]uuid.UUID, error) {
	// High limit so cycle detection sees every repo-type Source; a repository with
	// thousands of repo Sources is pathological.
	sources, _, err := m.archive.ListSources(repoID, core.SourceListOptions{
		Connector:   core.ConnectorRepo,
		ListOptions: core.ListOptions{Limit: 1000, SortBy: "created_at", Order: "desc"},
	})
	if err != nil {
		return nil, err
	}

	var targets []uuid.UUID
	for _, src := range sources {
		ref, _ := src.Config["sink_id"].(string)
		sinkID, err := uuid.Parse(ref)
		if err != nil {
			continue
		}
		sink, err := m.archive.GetSink(sinkID)
		if err != nil {
			continue
		}
		targets = append(targets, sink.RepoID)
	}

	return targets, nil
}
