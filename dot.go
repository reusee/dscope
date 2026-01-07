package dscope

import (
	"fmt"
	"io"
	"strings"
)

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
		typeName := typeIDToType(typeID).String()

		nodes[typeID] = struct{}{}
		nodeInfo[typeID] = fmt.Sprintf(
			"Type: %s\\nDefined By: %s",
			typeName,
			effectiveValue.typeInfo.DefType.String(),
		)

		for _, depID := range effectiveValue.typeInfo.Dependencies {
			if _, ok := scope.values.Load(depID); ok || isAlwaysProvided(depID) {
				nodes[depID] = struct{}{}
				edges[[2]_TypeID{depID, typeID}] = struct{}{}
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