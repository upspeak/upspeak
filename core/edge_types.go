package core

import "github.com/rs/xid"

// ReplyEdge returns a new Edge that denotes the target Node as a reply to the source Node.
// The weight of the Edge is set to 0.5.
func ReplyEdge(source, target xid.ID) Edge {
	return newEdge("Reply", source, target, "", 0.5)
}

// AnnotationEdge creates a new Edge that denotes the target Node is an annotation of the source Node.
// The weight of the Edge is set to 0.25.
func AnnotationEdge(source, target xid.ID) Edge {
	return newEdge("Annotation", source, target, "", 0.25)
}

// AttachmentEdge marks the target Node as an attachment of the source Node.
// The weight of the Edge is set to 0.3.
func AttachmentEdge(source, target xid.ID) Edge {
	return newEdge("Attachment", source, target, "", 0.3)
}

// ForkEdge creates a new Edge that denotes the target Node is a fork of the source Node.
// The weight of the Edge is set to 0.75.
func ForkEdge(source, target xid.ID) Edge {
	return newEdge("Fork", source, target, "", 0.75)
}
