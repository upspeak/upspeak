package core

const (
	// KindNode represents a standalone (isolated) Node.
	// In a threaded discussion model, these are Nodes that don't belong to any Thread in the graph.
	KindNode Kind = "Node"
)

// TextMetadata is the type used for a plaintext Node's metadata
type TextMetadata struct {
	// Fully qualified author/sender name, e.g., @alice:example.com, @bob:localhost
	Author string `json:"author"`
	// Version of the thread
	Version int `json:"version"`
	// Props is a map of additional properties that can be used to store custom metadata
	Props map[string]string `json:"properties,omitempty"`
}

// MarkdownMetadata is the type used for a Markdown Node's metadata
type MarkdownMetadata struct {
	// Fully qualified author/sender name, e.g., @alice:example.com, @bob:localhost
	Author string `json:"author"`
	// Version of the thread
	Version int `json:"version"`
	// FrontMatter is a map of key-value pairs that can be used to store any frontmatter
	// added to the markdown content
	FrontMatter map[string]string `json:"front_matter,omitempty"`
	// Props is a map of additional properties that can be used to store custom metadata
	Props map[string]string `json:"properties,omitempty"`
}

// TextContent represents the content of a text node
type TextContent struct {
	// Raw string. For Markdown nodes, store the Markdown content here.
	Text string `json:"text"`
	// Processed HTML string.
	HTML string `json:"html"`
}

// TextBody can be used for any node of type text, including plaintext, markdown, html, etc.
type TextBody struct {
	// ContentType describes Content-Type of the Content field. e.g., text/plain, text/markdown, text/html
	ContentType string `json:"content_type"`
	// Content is a map of key-value pairs that can be used to store the actual content of the node
	Content TextContent `json:"content"`
}

// TextNode returns a Node of KindNode with TextMetadata and TextBody
// Use this function to create a standalone Node.
// If you want to create a Node that is part of a Thread, use NewComment.
func TextNode(content TextContent, author string) Node[TextMetadata, TextBody] {
	metadata := TextMetadata{
		Author:  author,
		Version: 1,
		Props:   make(map[string]string),
	}
	body := TextBody{
		ContentType: "text/plain",
		Content:     content,
	}
	return NewNode(KindNode, metadata, body)
}

// MarkdownNode returns a Node of KindNode with MarkdownMetadata and TextBody
// Use this function to create a standalone Node.
// If you want to create a Node that is part of a Thread, use NewComment.
//
// Note: this function does not handle any markdown parsing. It is the caller's responsibility
// to ensure that the content is valid markdown.
func MarkdownNode(content TextContent, frontmatter map[string]string, author string) Node[MarkdownMetadata, TextBody] {
	metadata := MarkdownMetadata{
		Author:      author,
		Version:     1,
		FrontMatter: frontmatter,
		Props:       make(map[string]string),
	}
	body := TextBody{
		ContentType: "text/markdown",
		Content:     content,
	}
	return NewNode(KindNode, metadata, body)
}
