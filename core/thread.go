package core

import (
	"encoding/json"

	"github.com/rs/xid"
)

const (
	KindThread  Kind = "Thread"
	KindComment Kind = "Comment"
)

// ThreadMetadata represents thread-specific metadata
type ThreadMetadata struct {
	// Fully qualified author/sender name, e.g., @alice:example.com, @bob:localhost
	Author string `json:"author"`
	// Version of the thread
	Version int `json:"version"`
	// Props is a map of additional properties that can be used to store custom metadata
	Props map[string]string `json:"properties,omitempty"`
}

// ThreadBody represents the content specific to a thread
type ThreadBody struct {
	// Title for the thread
	Title string `json:"title"`
	// The actual content of the thread's body
	Content json.RawMessage `json:"content"`
}

// NewThread creates a new thread starting from an existing root node
// Thread is a higher order Node that links a list of Nodes through a parent-child relation. It extends core.Node.
// The first Node in a Thread is the root Node.
// Nodes in a thread can be created in reply to other Nodes in the same Thread.
// When a Thread begins from a Node that is already part of a Thread, the new Thread is a sub-thread of the existing Thread.
// These sub-threads are treated as branches of the main Thread. They can be merged back into the main Thread at a later time.
// Nodes in a Thread are ordered by their creation time and their ID
// Each Node in a thread can be of a different Datatype with its own Metadata and Body types.
// Thread mutation functions should return Nodes and Edges based on the function.
func NewThread(contentType string, body ThreadBody, author string) (Node, error) {
	metadata := ThreadMetadata{
		Author:  author,
		Version: 1,
		Props:   make(map[string]string),
	}
	jsonMetadata, err := json.Marshal(metadata)
	if err != nil {
		return Node{}, err
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return Node{}, err
	}

	return NewNode(KindThread, contentType, jsonMetadata, jsonBody), nil
}

// NewComment creates a new Node as a comment for a given thread id and creates a child edge
// between the comment and the thread.
func NewComment(thread_id xid.ID, contentType string, metadata, body json.RawMessage) (Node, Edge) {
	comment := NewNode(KindComment, contentType, metadata, body)
	edge := ChildEdge(comment.ID, thread_id)
	return comment, edge
}
