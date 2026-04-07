package jsonr

type graph struct {
	root *node
}

func newGraph() *graph {
	return &graph{root: newNode()}
}

func (g *graph) register(path []string, unmar unmarshaler) *unmarshaler {
	return g.root.register(path, unmar)
}
