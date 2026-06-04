package search

import "strconv"

// normaliseLimit clamps a requested page size into the allowed range, defaulting
// to 20 when unset or invalid.
func normaliseLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

// normaliseOffset clamps a requested offset into the allowed range, defaulting to
// 0 when negative.
func normaliseOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > 10000 {
		return 10000
	}
	return offset
}

// parseDepth parses a traversal depth query value, defaulting to 1 and clamping
// to [1, maxTraversalDepth].
func parseDepth(raw string) int {
	if raw == "" {
		return 1
	}
	d, err := strconv.Atoi(raw)
	if err != nil || d < 1 {
		return 1
	}
	if d > maxTraversalDepth {
		return maxTraversalDepth
	}
	return d
}

// normaliseDirection validates a traversal direction, defaulting to "both" for
// empty or unrecognised values.
func normaliseDirection(dir string) string {
	switch dir {
	case "outgoing", "incoming", "both":
		return dir
	default:
		return "both"
	}
}
