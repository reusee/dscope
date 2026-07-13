package dscope

import (
	"fmt"
	"io"
	"strings"
)

const TheoryOfScopeVisualization = `
dscope visualization theory:
- Graphs represent effective dependency resolution semantics.
- Built-in dependencies are shown as built-ins, even when a user supplies an
  ignored definition for the same type.
- Ignored definitions must not contribute labels or dependency edges.
`

func (scope Scope) ToDOT(w io.Writer) error {
	if _, err := io.WriteString(w, "digraph dscope {\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "  rankdir=LR;\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "  node [shape=box, style=filled, fillcolor=lightblue];\n"); err != nil {
		return err
	}

	nodes := make(map[_TypeID]struct{})
	edges := make(map[[2]_TypeID]struct{})
	nodeInfo := make(map[_TypeID]string)

	for typ := range scope.AllTypes() {
		id := getTypeID(typ)
		if isAlwaysProvided(id) {
			nodes[id] = struct{}{}
			nodeInfo[id] = fmt.Sprintf("Type: %s\\nBuilt-in", typ.String())
		}
	}

	for effectiveValue := range scope.values.IterValues() {
		typeID := effectiveValue.typeInfo.TypeID
		if isAlwaysProvided(typeID) {
			continue
		}
		typeName := typeIDToType(typeID).String()

		nodes[typeID] = struct{}{}
		nodeInfo[typeID] = fmt.Sprintf(
			"Type: %s\\nDefined By: %s",
			typeName,
			effectiveValue.typeInfo.DefType.String(),
		)

		for _, dependencyID := range effectiveValue.typeInfo.Dependencies {
			if _, ok := scope.values.Load(dependencyID); ok || isAlwaysProvided(dependencyID) {
				nodes[dependencyID] = struct{}{}
				edges[[2]_TypeID{dependencyID, typeID}] = struct{}{}
			}
		}
	}

	for id := range nodes {
		label := typeIDToType(id).String()
		if info, ok := nodeInfo[id]; ok {
			label = info
		}
		if _, err := fmt.Fprintf(w, "  \"%d\" [label=\"%s\"];\n", id, strings.ReplaceAll(label, "\"", "\\\"")); err != nil {
			return err
		}
	}

	for edge := range edges {
		if _, err := fmt.Fprintf(w, "  \"%d\" -> \"%d\";\n", edge[0], edge[1]); err != nil {
			return err
		}
	}

	if _, err := io.WriteString(w, "}\n"); err != nil {
		return err
	}

	return nil
}
