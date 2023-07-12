package graph_test

import (
	"errors"
	"testing"

	"github.com/zhamlin/chi-openapi/internal/graph"
	. "github.com/zhamlin/chi-openapi/internal/testing"
)

func TestGraph(t *testing.T) {
	g := graph.New[string]()
	aID := g.Add("A")
	bID := g.Add("B")
	dID := g.Add("D")
	cID := g.Add("C")

	MustMatch(t, g.AddEdges(aID, bID), nil)
	MustMatch(t, g.AddEdges(bID, cID), nil)

	sortedNodes, err := graph.TopologicalSort(g)
	MustMatch(t, err, nil)
	MustMatch(t, sortedNodes, []int{dID, aID, bID, cID})
}

func TestGraphCycleErr(t *testing.T) {
	g := graph.New[string]()

	dID := g.Add("D")
	cID := g.Add("C")
	aID := g.Add("A")
	bID := g.Add("B")

	MustMatch(t, g.AddEdges(aID, bID), nil)
	MustMatch(t, g.AddEdges(bID, cID), nil)
	MustMatch(t, g.AddEdges(cID, dID), nil)
	MustMatch(t, g.AddEdges(cID, aID), nil)

	_, err := graph.TopologicalSort(g)
	MustNotMatch(t, err, nil, "expected a cycle error")

	var errCycle graph.ErrNodeCycle
	if errors.As(err, &errCycle) {
		MustMatch(t, errCycle.SourceIndex, cID, "expected cycle index for node C")
	} else {
		t.Fatalf("expected a cycle error, got: %T", err)
	}
}

type String string

func (s String) String() string {
	return string(s)
}

func (s String) NodeShape() string {
	return "box"
}

// TODO: make deterministic
/*
func TestGraphDot(t *testing.T) {
	g := graph.New[String]()

	dID := g.Add("D")
	cID := g.Add("C")
	aID := g.Add("A")
	bID := g.Add("B")

	MustMatch(t, g.AddEdges(aID, bID), nil)
	MustMatch(t, g.AddEdges(bID, cID), nil)
	MustMatch(t, g.AddEdges(cID, dID), nil)
	MustMatch(t, g.AddEdges(cID, aID), nil)

	dotText := graph.GraphToDot(g)
	MustMatch(t, dotText, `digraph D {
  node [shape=box]; n0 [label="(n0) D"]; n1 [label="(n1) C"]; n2 [label="(n2) A"]; n3 [label="(n3) B"];
  n2 -> {n3}
  n3 -> {n1}
  n1 -> {n0,n2}
}`)
}
*/
