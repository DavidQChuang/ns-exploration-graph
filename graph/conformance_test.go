package graph_test

import (
	"ns-exploration-graph/graph"
	"ns-exploration-graph/graph/conformance"
	"ns-exploration-graph/graph/store/memory"
	"testing"
)

func TestMemoryConformance(t *testing.T) {
	conformance.Run(t, func(t *testing.T) graph.Store { return memory.New() })
}
