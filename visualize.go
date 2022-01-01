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
			if value.Kind != reflect.Func {
				continue
			}
			defValue := reflect.ValueOf(value.Def)
			defType := defValue.Type()
			for i := 0; i < defType.NumIn(); i++ {
				in := defType.In(i)
				for j := 0; j < defType.NumOut(); j++ {
					out := defType.Out(j)
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
