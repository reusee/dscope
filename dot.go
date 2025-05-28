package dscope

import (
	"fmt"
	"io"
	"strings"
)

// ToDOT generates a DOT language representation of the scope's dependency graph.
// This output can be used with tools like Graphviz to visualize the scope's structure.
// It shows the effective definition for each type and its direct dependencies.
func (scope Scope) ToDOT(w io.Writer) error {
	if _, err := io.WriteString(w, "digraph dscope {\n"); err != nil {
		return err
	}
	// Left-to-right layout often works well for dependencies
	if _, err := io.WriteString(w, "  rankdir=LR;\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "  node [shape=box, style=filled, fillcolor=lightblue];\n"); err != nil {
		return err
	}

	nodes := make(map[_TypeID]struct{})
	edges := make(map[[2]_TypeID]struct{}) // [from, to]
	nodeInfo := make(map[_TypeID]string)   // Extra info for node labels

	// Iterate through all *effective* values in the scope
	for effectiveValue := range scope.values.IterValues() {
		// Use the first value (top of stack) as the effective one
		typeID := effectiveValue.typeInfo.TypeID
		typeName := typeIDToType(typeID).String()

		// Skip built-in types unless they have dependencies (unlikely but possible)
		if isAlwaysProvided(typeID) && len(effectiveValue.typeInfo.Dependencies) == 0 {
			continue
		}

		nodes[typeID] = struct{}{}
		nodeInfo[typeID] = fmt.Sprintf(
			"Type: %s\\nDefined By: %s",
			typeName,
			effectiveValue.typeInfo.DefType.String(), // Show the func/ptr type that defines it
		)

		// Add edges for dependencies
		for _, depID := range effectiveValue.typeInfo.Dependencies {
			// Skip built-in types as sources unless explicitly defined
			if isAlwaysProvided(depID) {
				// Check if the built-in type *is* actually defined in the scope explicitly
				if _, defined := scope.values.Load(depID); !defined {
					continue
				}
			}
			nodes[depID] = struct{}{} // Ensure dependency node exists
			edges[[2]_TypeID{depID, typeID}] = struct{}{}
		}
	}

	// Write nodes
	for id := range nodes {
		label := typeIDToType(id).String()
		if info, ok := nodeInfo[id]; ok {
			label = info // Use detailed label if available
		}
		if _, err := fmt.Fprintf(w, "  \"%d\" [label=\"%s\"];\n", id, strings.ReplaceAll(label, "\"", "\\\"")); err != nil {
			return err
		}
	}

	// Write edges
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
