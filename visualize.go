package dscope

import (
	"io"
	"reflect"
	"sync"

	"github.com/emicklei/dot"
)

func (s Scope) Visualize(w io.Writer) error {

	edges := make(map[[2]reflect.Type]bool)
	if err := s.values.Range(func(values []_Value) error {

		for _, value := range values {
			if value.DefType.Kind() != reflect.Func {
				continue
			}
			for i := 0; i < value.DefType.NumIn(); i++ {
				in := value.DefType.In(i)
				for j := 0; j < value.DefType.NumOut(); j++ {
					out := value.DefType.Out(j)
					edges[[2]reflect.Type{out, in}] = true
				}
			}
		}

		return nil
	}); err != nil {
		return we(err)
	}

	g := dot.NewGraph(dot.Directed)
	var nodes sync.Map
	getNode := func(t reflect.Type) dot.Node {
		if v, ok := nodes.Load(t); ok {
			return v.(dot.Node)
		}
		node := g.Node(t.String())
		nodes.Store(t, node)
		return node
	}

	for edge := range edges {
		g.Edge(
			getNode(edge[0]),
			getNode(edge[1]),
		)
	}

	_, err := w.Write([]byte(g.String()))
	if err != nil {
		return we(err)
	}

	return nil
}
