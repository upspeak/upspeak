package core

const KindThread Kind = "thread"

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
	// Content-Type of the Content field
	ContentType string `json:"content_type"`
	// The actual content of the thread's body
	Content map[string]any `json:"content"`
}

// Thread is a higher order Node that links a list of Nodes through a parent-child relation. It extends core.Node.
// The first Node in a Thread is the root Node.
// Nodes in a thread can be created in reply to other Nodes in the same Thread.
// When a Thread begins from a Node that is already part of a Thread, the new Thread is a sub-thread of the existing Thread.
// These sub-threads are treated as branches of the main Thread. They can be merged back into the main Thread at a later time.
// Nodes in a Thread are ordered by their creation time and their ID
// Each Node in a thread can be of a different Datatype with its own Metadata and Body types.
// Thread mutation functions should return Nodes and Edges based on the function.
type Thread struct {
	Node[ThreadMetadata, ThreadBody]
}

// NewThread creates a new thread starting from an existing root node
func NewThread(body ThreadBody, author string) *Thread {
	metadata := ThreadMetadata{
		Author:  author,
		Version: 1,
		Props:   make(map[string]string),
	}

	return &Thread{
		Node: *NewNode(KindThread, metadata, body),
	}
}

// SetProps adds a property to the thread metadata
func (t *Thread) SetProps(key, value string) *Thread {
	t.Metadata.Props[key] = value
	return t
}

// WithContent sets the content of the thread body
func (t *Thread) WithContent(content_type string, content map[string]any) *Thread {
	t.Body.ContentType = content_type
	t.Body.Content = content
	return t
}
