package core

import "github.com/rs/xid"

// ReplyEdge returns a new Edge denoting that the source Node is a reply to the target Node.
// The weight of the Edge is set to 0.5.
func ReplyEdge(reply_node, in_reply_to_node xid.ID) Edge {
	return newEdge("Reply", reply_node, in_reply_to_node, "", 0.5)
}

// ChildEdge returns a new Edge that creates a "belongs to" relationship between the source and target Nodes.
// The source node is the child of the target node.
// The weight of the Edge is set to 1.0.
func ChildEdge(child_node, parent_node xid.ID) Edge {
	return newEdge("Child", child_node, parent_node, "", 1.0)
}

// AnnotationEdge creates a new Edge that denotes the source node is an annotation of the target node.
// The weight of the Edge is set to 0.25.
func AnnotationEdge(annotation_node, target_node xid.ID) Edge {
	return newEdge("Annotation", annotation_node, target_node, "", 0.25)
}

// AttachmentEdge creates a new Edge that denotes the source node is an attachment of the target node.
// The weight of the Edge is set to 0.5.
func AttachmentEdge(attachment_node, belongs_to_node xid.ID) Edge {
	return newEdge("Attachment", attachment_node, belongs_to_node, "", 0.5)
}

// ForkEdge creates a new Edge that denotes source node is a fork of the target node.
// The weight of the Edge is set to 0.5.
func ForkEdge(forked_node, origin_node xid.ID) Edge {
	return newEdge("Fork", forked_node, origin_node, "", 0.5)
}
