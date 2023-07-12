package graph

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/zhamlin/chi-openapi/internal"
)

type sortMark int

const (
	sortMarkNone sortMark = iota
	sortMarkPermanent
	sortMarkTemporary
)

func TopologicalSort[T any](g *Graph[T]) ([]int, error) {
	nodeMarks := map[int]sortMark{}
	sortedNodes := make([]int, 0, len(g.nodes))
	for i := 0; i < len(g.nodes); i++ {
		if err := visit(g, i, &sortedNodes, nodeMarks); err != nil {
			return sortedNodes, err
		}
	}
	return sortedNodes, nil
}

type ErrNodeCycle struct {
	SourceIndex int
	Indexes     []int
}

func (e ErrNodeCycle) FirstIndex() (int, bool) {
	if len(e.Indexes) >= 1 {
		return e.Indexes[0], true
	}
	return 0, false
}

func (e ErrNodeCycle) Error() string {
	return fmt.Sprintf("source: %d, path: %v", e.SourceIndex, e.Indexes)
}

func visit[T any](
	g *Graph[T],
	nodeIndex int,
	sortedNodes *[]int,
	nodeMarks map[int]sortMark,
) error {
	switch nodeMarks[nodeIndex] {
	case sortMarkPermanent:
		return nil
	case sortMarkTemporary:
		return ErrNodeCycle{
			SourceIndex: nodeIndex,
			Indexes:     []int{},
		}
		// return fmt.Errorf("cycle detected, node index: %v", nodeIndex)
	}

	nodeMarks[nodeIndex] = sortMarkTemporary

	for edgeIndex := range g.EdgesFromTo[nodeIndex] {
		if err := visit(g, edgeIndex, sortedNodes, nodeMarks); err != nil {
			var errNodeCycle ErrNodeCycle
			if errors.As(err, &errNodeCycle) {
				errNodeCycle.Indexes = append(errNodeCycle.Indexes, nodeIndex)
				return errNodeCycle
			}
			return err
		}
	}
	nodeMarks[nodeIndex] = sortMarkPermanent

	*sortedNodes = append(
		[]int{nodeIndex},
		*sortedNodes...)
	return nil
}

func New[T any]() *Graph[T] {
	return &Graph[T]{
		nodes:       []T{},
		EdgesFromTo: map[int]internal.Set[int]{},
		EdgesToFrom: map[int]internal.Set[int]{},
	}
}

// Append only
type Graph[T any] struct {
	nodes []T
	// every edge from this index to other indexes
	EdgesFromTo map[int]internal.Set[int]
	// every edge from other indexes to this index
	EdgesToFrom map[int]internal.Set[int]
}

func (g *Graph[T]) isValidIndex(idx int) error {
	l := len(g.nodes) - 1
	if idx > l || idx < 0 {
		return fmt.Errorf("invalid index %d; expected 0 <= index <= %d", idx, l)
	}
	return nil
}

func (g *Graph[T]) Add(n T) int {
	g.nodes = append(g.nodes, n)
	return len(g.nodes) - 1
}

func (g *Graph[T]) Get(idx int) (*T, error) {
	if err := g.isValidIndex(idx); err != nil {
		return nil, err
	}
	return &g.nodes[idx], nil
}

// AddEdges connects (one way) fromIdx to the given indexes.
func (g *Graph[T]) AddEdges(fromIdx int, toIndexes ...int) error {
	if err := g.isValidIndex(fromIdx); err != nil {
		return err
	}
	for _, idx := range toIndexes {
		if err := g.isValidIndex(idx); err != nil {
			return err
		}
	}

	addToSet := func(m map[int]internal.Set[int], idx int, val ...int) {
		s, has := m[idx]
		if !has {
			s = internal.NewSet[int]()
		}
		for _, k := range val {
			s.Add(k)
		}
		m[idx] = s
	}

	// add edges from fromIdx to all toIndexes
	addToSet(g.EdgesFromTo, fromIdx, toIndexes...)

	// add edges from each toIndexes
	for _, toIdx := range toIndexes {
		addToSet(g.EdgesToFrom, toIdx, fromIdx)
	}
	return nil
}

type DotNode interface {
	String() string
	NodeShape() string
}

type dotNodeInfo struct {
	label string
	id    string
}

func (n dotNodeInfo) String() string {
	if n.label != "" {
		return fmt.Sprintf(`%s [label="(%s) %s"]`, n.id, n.id, n.label)
	}
	return n.id
}

func GraphToDot[T DotNode](g *Graph[T]) string {
	// list all nodesByShape without edges
	nodesByShape := map[string][]dotNodeInfo{}
	for idx, n := range g.nodes {
		label := n.String()
		// only show nodes with a label
		if label == "" {
			continue
		}

		shapes, has := nodesByShape[n.NodeShape()]
		if !has {
			shapes = []dotNodeInfo{}
		}
		shapes = append(shapes, dotNodeInfo{
			label: n.String(),
			id:    fmt.Sprintf("n%d", idx),
		})
		nodesByShape[n.NodeShape()] = shapes
	}

	// add edges
	edgesFromTo := []string{}
	for from, to := range g.EdgesFromTo {
		toNodes := []string{}
		for _, idx := range to.Items() {
			// only show nodes with a label
			if g.nodes[idx].String() != "" {
				toNodes = append(toNodes, fmt.Sprintf("n%d", idx))
			}
		}

		fromStr := fmt.Sprintf("n%d", from)
		edgesFromTo = append(edgesFromTo, fmt.Sprintf(
			"%s -> {%v}", fromStr, strings.Join(toNodes, ","),
		))
	}

	nodes := []string{}
	for shape, n := range nodesByShape {
		nodeLine := ""
		for _, info := range n {
			nodeLine += info.String() + "; "
		}
		// TODO: switch to one node per line; ;
		// "<node-id>" [ label="...",shape="hexagon",style="filled",color="green"];
		nodes = append(nodes, fmt.Sprintf("node [shape=%s]; %s", shape, nodeLine))
	}

	dotText := `digraph D {
  %s
  %s
}`
	// sort nodes to keep the output deterministic
	sort.Strings(nodes)
	dotText = fmt.Sprintf(dotText,
		strings.Join(nodes, "\n"),
		strings.Join(edgesFromTo, "\n  "),
	)
	return dotText
}
