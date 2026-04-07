package jsonr

type node struct {
	children map[string]*node
	unmar    unmarshaler
}

func (n *node) register(path []string, unmar unmarshaler) *unmarshaler {
	if len(path) == 0 {
		n.unmar = unmar
		return &n.unmar
	}
	child := n.children[path[0]]
	if child == nil {
		child = newNode()
		n.children[path[0]] = child
	}
	return child.register(path[1:], unmar)
}

// child looks up a trie edge for one path segment: exact key/index, else "*".
func (n *node) child(seg string) *node {
	if c := n.children[seg]; c != nil {
		return c
	}
	return n.children["*"]
}

func newNode() *node {
	return &node{
		children: make(map[string]*node),
		unmar:    nil,
	}
}
