package realtime

import "testing"

func TestParseChannel(t *testing.T) {
	tests := []struct {
		name      string
		channel   string
		wantKind  channelKind
		wantRepo  string
		wantEnt   string
		wantError bool
	}{
		{"repo events", "repos.research.events", channelRepoEvents, "research", "", false},
		{"node", "repos.research.nodes.NODE-42", channelNode, "research", "NODE-42", false},
		{"thread", "repos.research.threads.THREAD-7", channelThread, "research", "THREAD-7", false},
		{"rule actions stub", "repos.research.rules.RULE-3.actions", channelRuleActions, "research", "RULE-3", false},
		{"job stub", "jobs.JOB-9", channelJob, "", "JOB-9", false},
		{"sync stub", "sync", channelSync, "", "", false},
		{"empty", "", 0, "", "", true},
		{"unknown root", "foo.bar", 0, "", "", true},
		{"repo missing tail", "repos.research", 0, "", "", true},
		{"node missing ref", "repos.research.nodes", 0, "", "", true},
		{"rule missing actions suffix", "repos.research.rules.RULE-3", 0, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChannel(tt.channel)
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tt.channel)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.kind != tt.wantKind {
				t.Errorf("kind: got %v, want %v", got.kind, tt.wantKind)
			}
			if got.repoRef != tt.wantRepo {
				t.Errorf("repoRef: got %q, want %q", got.repoRef, tt.wantRepo)
			}
			if got.entityRef != tt.wantEnt {
				t.Errorf("entityRef: got %q, want %q", got.entityRef, tt.wantEnt)
			}
		})
	}
}
