package core

import (
	"testing"
)

func TestTextNode(t *testing.T) {
	tests := []struct {
		content TextContent
		author  string
	}{
		{TextContent{Text: "Hello, World!", HTML: "<p>Hello, World!</p>"}, "@alice:example.com"},
		{TextContent{Text: "Another text", HTML: "<p>Another text</p>"}, "@bob:localhost"},
	}

	for _, tt := range tests {
		node := TextNode(tt.content, tt.author)
		if node.Kind != KindNode {
			t.Errorf("expected KindNode, got %v", node.Kind)
		}
		if node.Metadata.Author != tt.author {
			t.Errorf("expected author %v, got %v", tt.author, node.Metadata.Author)
		}
		if node.Body.Content.Text != tt.content.Text {
			t.Errorf("expected text %v, got %v", tt.content.Text, node.Body.Content.Text)
		}
		if node.Body.Content.HTML != tt.content.HTML {
			t.Errorf("expected HTML %v, got %v", tt.content.HTML, node.Body.Content.HTML)
		}
		if node.Body.ContentType != "text/plain" {
			t.Errorf("expected content type text/plain, got %v", node.Body.ContentType)
		}
	}
}

func TestMarkdownNode(t *testing.T) {
	tests := []struct {
		content     TextContent
		frontmatter map[string]string
		author      string
	}{
		{
			content:     TextContent{Text: "# Hello, World!", HTML: "<h1>Hello, World!</h1>"},
			frontmatter: map[string]string{"title": "Hello World", "date": "2024-11-15"},
			author:      "@alice:example.com",
		},
		{
			content:     TextContent{Text: "## Another markdown", HTML: "<h2>Another markdown</h2>"},
			frontmatter: map[string]string{"title": "Another Markdown", "date": "2024-11-16"},
			author:      "@bob:localhost",
		},
	}

	for _, tt := range tests {
		node := MarkdownNode(tt.content, tt.frontmatter, tt.author)
		if node.Kind != KindNode {
			t.Errorf("expected KindNode, got %v", node.Kind)
		}
		if node.Metadata.Author != tt.author {
			t.Errorf("expected author %v, got %v", tt.author, node.Metadata.Author)
		}
		if node.Metadata.FrontMatter["title"] != tt.frontmatter["title"] {
			t.Errorf("expected frontmatter title %v, got %v", tt.frontmatter["title"], node.Metadata.FrontMatter["title"])
		}
		if node.Metadata.FrontMatter["date"] != tt.frontmatter["date"] {
			t.Errorf("expected frontmatter date %v, got %v", tt.frontmatter["date"], node.Metadata.FrontMatter["date"])
		}
		if node.Body.Content.Text != tt.content.Text {
			t.Errorf("expected text %v, got %v", tt.content.Text, node.Body.Content.Text)
		}
		if node.Body.Content.HTML != tt.content.HTML {
			t.Errorf("expected HTML %v, got %v", tt.content.HTML, node.Body.Content.HTML)
		}
		if node.Body.ContentType != "text/markdown" {
			t.Errorf("expected content type text/markdown, got %v", node.Body.ContentType)
		}
	}
}
