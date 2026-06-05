package realtime

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/upspeak/upspeak/core"
)

// errSubscriptionLimit is returned when a connection exceeds maxSubscriptions.
var errSubscriptionLimit = fmt.Errorf("subscription limit reached")

// defaultOwnerID scopes repo-ref resolution to the single local user. Multi-user
// ownership is deferred (see Phase 5/6 scope), so realtime resolves repo refs
// under the same fixed owner the repo module uses.
var defaultOwnerID = uuid.MustParse("00000000-0000-7000-8000-000000000001")

// refResolver is the narrow slice of core.Archive the realtime module needs to
// turn client channel refs (UUID, short ID, or slug) into canonical UUIDs.
// core.Archive satisfies it.
type refResolver interface {
	ResolveRepoRef(ownerID uuid.UUID, ref string) (*core.Repository, error)
	ResolveRef(repoID uuid.UUID, ref string) (uuid.UUID, string, error)
}

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
	channel  string // raw channel string; the map key and echoed to client
	kind     channelKind
	repoID   uuid.UUID // resolved repo UUID for repo-scoped channels
	entityID uuid.UUID // resolved node/thread UUID where applicable
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

// resolveChannel turns a parsed channel into an active subscription, resolving
// any repo and entity refs to canonical UUIDs and attaching the optional filter.
// It returns an error when a ref cannot be resolved, which the caller surfaces
// to the client as an invalid_channel error.
func resolveChannel(pc parsedChannel, filter *subFilter, r refResolver) (*subscription, error) {
	sub := &subscription{channel: pc.raw, kind: pc.kind, filter: filter}

	// Stub channels (jobs, sync) carry no backing events yet, so they need no
	// ref resolution; they are accepted structurally and never match.
	if pc.kind == channelSync || pc.kind == channelJob {
		return sub, nil
	}

	repo, err := r.ResolveRepoRef(defaultOwnerID, pc.repoRef)
	if err != nil {
		return nil, fmt.Errorf("resolve repo %q: %w", pc.repoRef, err)
	}
	sub.repoID = repo.ID

	if pc.entityRef == "" {
		return sub, nil // repo-events channel; nothing more to resolve.
	}

	id, typ, err := r.ResolveRef(repo.ID, pc.entityRef)
	if err != nil {
		return nil, fmt.Errorf("resolve ref %q: %w", pc.entityRef, err)
	}
	if want := expectedEntityType(pc.kind); want != "" && typ != want {
		return nil, fmt.Errorf("ref %q is a %s, not a %s", pc.entityRef, typ, want)
	}
	sub.entityID = id
	return sub, nil
}

// expectedEntityType returns the entity type a channel kind's ref must resolve
// to, or "" when no type constraint applies. A rule-actions ref must resolve to
// a rule even though delivery on that channel is still a stub, so a wrong-typed
// ref (e.g. a node short ID) is rejected at subscribe time rather than silently
// accepted as a dead subscription.
func expectedEntityType(kind channelKind) string {
	switch kind {
	case channelNode:
		return "node"
	case channelThread:
		return "thread"
	case channelRuleActions:
		return "rule"
	default:
		return ""
	}
}
