package core

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestEdgeProvenanceRoundTrip(t *testing.T) {
	sid := uuid.New()
	ext := "ext-42"
	e := Edge{ID: uuid.New(), SourceID: &sid, ExternalID: &ext}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Edge
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SourceID == nil || *got.SourceID != sid || got.ExternalID == nil || *got.ExternalID != ext {
		t.Fatalf("provenance not preserved: %+v", got)
	}
}

func TestIngestBatchHasEdges(t *testing.T) {
	b := IngestBatch{Edges: []IngestEdge{{ExternalID: "e1", SourceExternalID: "n1", TargetExternalID: "n2", Type: "reply"}}}
	if len(b.Edges) != 1 || b.Edges[0].SourceExternalID != "n1" {
		t.Fatalf("edges field missing/wrong: %+v", b)
	}
}
