package core

import (
	"testing"

	"github.com/google/uuid"
)

func TestNewID(t *testing.T) {
	id1 := NewID()
	id2 := NewID()

	if id1 == uuid.Nil {
		t.Error("NewID returned nil UUID")
	}
	if id1 == id2 {
		t.Error("Two calls to NewID returned the same UUID")
	}

	// UUID v7 has version nibble = 7.
	if id1.Version() != 7 {
		t.Errorf("Expected UUID version 7, got %d", id1.Version())
	}
}

func TestNewID_TimeOrdered(t *testing.T) {
	ids := make([]uuid.UUID, 100)
	for i := range ids {
		ids[i] = NewID()
	}
	// UUID v7 is time-ordered: lexicographic ordering should match creation order.
	for i := 1; i < len(ids); i++ {
		if ids[i].String() < ids[i-1].String() {
			t.Errorf("UUID %d (%s) is less than UUID %d (%s) — not time-ordered",
				i, ids[i], i-1, ids[i-1])
		}
	}
}

func TestFormatShortID(t *testing.T) {
	tests := []struct {
		prefix   string
		seq      int
		expected string
	}{
		{"NODE", 1, "NODE-1"},
		{"NODE", 42, "NODE-42"},
		{"REPO", 1, "REPO-1"},
		{"EDGE", 100, "EDGE-100"},
		{"SCHED", 7, "SCHED-7"},
	}

	for _, tt := range tests {
		got := FormatShortID(tt.prefix, tt.seq)
		if got != tt.expected {
			t.Errorf("FormatShortID(%q, %d) = %q, want %q", tt.prefix, tt.seq, got, tt.expected)
		}
	}
}

func TestParseShortID(t *testing.T) {
	tests := []struct {
		input      string
		wantPrefix string
		wantSeq    int
		wantErr    bool
	}{
		{"NODE-42", "NODE", 42, false},
		{"REPO-1", "REPO", 1, false},
		{"EDGE-100", "EDGE", 100, false},
		{"THREAD-7", "THREAD", 7, false},
		{"ANNO-23", "ANNO", 23, false},
		{"FILTER-3", "FILTER", 3, false},
		{"SRC-5", "SRC", 5, false},
		{"SINK-2", "SINK", 2, false},
		{"RULE-8", "RULE", 8, false},
		{"SCHED-4", "SCHED", 4, false},
		{"JOB-109", "JOB", 109, false},
		{"USER-1", "USER", 1, false},
		// Invalid cases.
		{"INVALID-1", "", 0, true},   // Unknown prefix
		{"NODE", "", 0, true},        // No sequence
		{"NODE-0", "", 0, true},      // Zero sequence
		{"NODE--1", "", 0, true},     // Negative sequence
		{"NODE-abc", "", 0, true},    // Non-numeric sequence
		{"", "", 0, true},            // Empty string
		{"-1", "", 0, true},         // No prefix
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			prefix, seq, err := ParseShortID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseShortID(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseShortID(%q) unexpected error: %v", tt.input, err)
				return
			}
			if prefix != tt.wantPrefix {
				t.Errorf("ParseShortID(%q) prefix = %q, want %q", tt.input, prefix, tt.wantPrefix)
			}
			if seq != tt.wantSeq {
				t.Errorf("ParseShortID(%q) seq = %d, want %d", tt.input, seq, tt.wantSeq)
			}
		})
	}
}

func TestIsShortID(t *testing.T) {
	if !IsShortID("NODE-42") {
		t.Error("Expected NODE-42 to be a valid short ID")
	}
	if IsShortID("not-a-short-id") {
		t.Error("Expected 'not-a-short-id' to not be a valid short ID")
	}
	if IsShortID("01964d2e-7c00-7000-8000-000000000042") {
		t.Error("Expected UUID to not be a valid short ID")
	}
}

func TestIsValidSlug(t *testing.T) {
	tests := []struct {
		slug  string
		valid bool
	}{
		{"research", true},
		{"ai-governance", true},
		{"my-repo-1", true},
		{"a", true},
		{"123", true},
		{"a-b-c", true},
		// Invalid cases.
		{"", false},
		{"-starts-with-dash", false},
		{"UPPERCASE", false},
		{"has spaces", false},
		{"has_underscores", false},
		{"has.dots", false},
		{"this-slug-is-way-too-long-for-the-maximum-allowed-characters", false},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			got := IsValidSlug(tt.slug)
			if got != tt.valid {
				t.Errorf("IsValidSlug(%q) = %v, want %v", tt.slug, got, tt.valid)
			}
		})
	}
}

func TestIsValidUUID(t *testing.T) {
	if !IsValidUUID("01964d2e-7c00-7000-8000-000000000042") {
		t.Error("Expected valid UUID to pass")
	}
	if !IsValidUUID(NewID().String()) {
		t.Error("Expected generated UUID to pass")
	}
	if IsValidUUID("not-a-uuid") {
		t.Error("Expected invalid string to fail")
	}
	if IsValidUUID("NODE-42") {
		t.Error("Expected short ID to fail UUID check")
	}
}
