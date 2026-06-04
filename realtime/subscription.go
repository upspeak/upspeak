package realtime

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// channelKind enumerates the channel families a client can subscribe to.
type channelKind int

const (
	channelRepoEvents  channelKind = iota // repos.{repo}.events
	channelNode                           // repos.{repo}.nodes.{node}
	channelThread                         // repos.{repo}.threads.{thread}
	channelRuleActions                    // repos.{repo}.rules.{rule}.actions (stub)
	channelJob                            // jobs.{job} (stub)
	channelSync                           // sync (stub)
)

// parsedChannel is the structural result of parsing a client channel string,
// before any ref is resolved to a UUID.
type parsedChannel struct {
	kind      channelKind
	raw       string // original channel string, echoed back to the client
	repoRef   string // repo ref for repo-scoped channels
	entityRef string // node/thread/rule/job ref; empty for events and sync
}

// subscription is a resolved, active subscription stored on a connection.
type subscription struct {
	channel  string      // raw channel string; the map key and echoed to client
	kind     channelKind
	repoID   uuid.UUID   // resolved repo UUID for repo-scoped channels
	entityID uuid.UUID   // resolved node/thread UUID where applicable
	filter   *subFilter
}

// parseChannel parses a client channel string into its structural parts. It
// validates shape only; ref resolution happens later in resolveChannel.
func parseChannel(channel string) (parsedChannel, error) {
	if channel == "" {
		return parsedChannel{}, fmt.Errorf("empty channel")
	}
	if channel == "sync" {
		return parsedChannel{kind: channelSync, raw: channel}, nil
	}
	parts := strings.Split(channel, ".")
	switch parts[0] {
	case "jobs":
		if len(parts) != 2 || parts[1] == "" {
			return parsedChannel{}, fmt.Errorf("invalid job channel %q", channel)
		}
		return parsedChannel{kind: channelJob, raw: channel, entityRef: parts[1]}, nil
	case "repos":
		return parseRepoChannel(channel, parts)
	default:
		return parsedChannel{}, fmt.Errorf("unknown channel root %q", parts[0])
	}
}

// parseRepoChannel handles the repos.{repo_ref}.* family.
func parseRepoChannel(channel string, parts []string) (parsedChannel, error) {
	// parts[0] == "repos"; parts[1] == repo ref.
	if len(parts) < 3 || parts[1] == "" {
		return parsedChannel{}, fmt.Errorf("invalid repo channel %q", channel)
	}
	repoRef := parts[1]
	switch {
	case len(parts) == 3 && parts[2] == "events":
		return parsedChannel{kind: channelRepoEvents, raw: channel, repoRef: repoRef}, nil
	case len(parts) == 4 && parts[2] == "nodes" && parts[3] != "":
		return parsedChannel{kind: channelNode, raw: channel, repoRef: repoRef, entityRef: parts[3]}, nil
	case len(parts) == 4 && parts[2] == "threads" && parts[3] != "":
		return parsedChannel{kind: channelThread, raw: channel, repoRef: repoRef, entityRef: parts[3]}, nil
	case len(parts) == 5 && parts[2] == "rules" && parts[3] != "" && parts[4] == "actions":
		return parsedChannel{kind: channelRuleActions, raw: channel, repoRef: repoRef, entityRef: parts[3]}, nil
	default:
		return parsedChannel{}, fmt.Errorf("invalid repo channel %q", channel)
	}
}
