package core

// Archive is an interface that defines the methods to store and load a repo
type Archive interface {
	Load() (*Repo, error)
	FetchNode(id string) (*Node, error)
	UpdateNode(node *Node) error
	FetchEdge(id string) (*Edge, error)
	UpdateEdge(edge *Edge) error
	CreateNode(node *Node) error
	CreateEdge(edge *Edge) error
}
