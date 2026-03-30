package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Entity prefix constants for short ID generation.
const (
	PrefixRepo       = "REPO"
	PrefixNode       = "NODE"
	PrefixEdge       = "EDGE"
	PrefixThread     = "THREAD"
	PrefixAnnotation = "ANNO"
	PrefixFilter     = "FILTER"
	PrefixSource     = "SRC"
	PrefixSink       = "SINK"
	PrefixRule       = "RULE"
	PrefixSchedule   = "SCHED"
	PrefixJob        = "JOB"
	PrefixUser       = "USER"
)

// EntityPrefixToType maps short ID prefixes to entity type names.
var EntityPrefixToType = map[string]string{
	PrefixRepo:       "repo",
	PrefixNode:       "node",
	PrefixEdge:       "edge",
	PrefixThread:     "thread",
	PrefixAnnotation: "annotation",
	PrefixFilter:     "filter",
	PrefixSource:     "source",
	PrefixSink:       "sink",
	PrefixRule:       "rule",
	PrefixSchedule:   "schedule",
	PrefixJob:        "job",
	PrefixUser:       "user",
}

// slugRegex validates repository slugs: lowercase alphanumeric + hyphens, 1-32 chars.
var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// NewID generates a new UUID v7 (time-ordered).
func NewID() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}

// FormatShortID constructs a short ID string from a prefix and sequence number.
//
// Example: FormatShortID("NODE", 42) returns "NODE-42".
func FormatShortID(prefix string, seq int) string {
	return fmt.Sprintf("%s-%d", prefix, seq)
}

// ParseShortID extracts the prefix and sequence number from a short ID string.
// Returns the prefix, sequence, and any parsing error.
//
// Example: ParseShortID("NODE-42") returns ("NODE", 42, nil).
func ParseShortID(s string) (string, int, error) {
	idx := strings.LastIndex(s, "-")
	if idx < 1 {
		return "", 0, fmt.Errorf("invalid short ID format: %s", s)
	}

	prefix := s[:idx]
	if _, ok := EntityPrefixToType[prefix]; !ok {
		return "", 0, fmt.Errorf("unknown entity prefix: %s", prefix)
	}

	seq, err := strconv.Atoi(s[idx+1:])
	if err != nil || seq < 1 {
		return "", 0, fmt.Errorf("invalid sequence number in short ID: %s", s)
	}

	return prefix, seq, nil
}

// IsShortID checks whether the given string looks like a valid short ID
// (has a known prefix followed by a dash and positive integer).
func IsShortID(s string) bool {
	_, _, err := ParseShortID(s)
	return err == nil
}

// IsValidSlug checks whether a string is a valid repository slug.
// Rules: lowercase alphanumeric + hyphens, 1-32 characters, starts with alphanumeric.
func IsValidSlug(s string) bool {
	return slugRegex.MatchString(s)
}

// IsValidUUID checks whether a string is a valid UUID.
func IsValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}
