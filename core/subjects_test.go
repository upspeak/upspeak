package core

import (
	"testing"

	"github.com/google/uuid"
)

func TestSinkSubject(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	got := SinkSubject(id, EventNodeCreated)
	want := "sink.11111111-1111-1111-1111-111111111111.events.NodeCreated"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
