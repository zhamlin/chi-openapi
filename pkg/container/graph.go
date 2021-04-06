package container

import (
	"errors"
	"fmt"
	"reflect"
)

// Edge represents the relationship between two types
type Edge struct {
	From reflect.Type
	To   reflect.Type

	hasSpecialInType bool
}

type Edges []*Edge

func (e Edge) Equal(edge Edge) bool {
	return e.From == edge.From &&
		e.To == edge.To
}

func (e Edges) Contains(t reflect.Type) bool {
	for _, edge := range e {
		if edge.From == t {
			return true
		}
	}
	return false
}

// Vertex represents a reflect.Type and a function that is used to create it.
type Vertex struct {
	// function used to create this type
	fn reflect.Value
	// the reflect.Type value that the function will return
	Typ reflect.Type
	// the position of the error, if any, from the functions returns
	errorOutLocation int

	// slice of pointers to the graphs edges relating to this vertex
	// each edge represents a dependency of the function, incoming being
	// the arguments required, and outgoing for the values provided from this function
	OutgoingEdges Edges
	IncomingEdges Edges
}

func NewGraph() *Graph {
	return &Graph{
		Edges:    []Edge{},
		Vertexes: map[reflect.Type]*Vertex{},
	}
}

type Graph struct {
	Edges    []Edge
	Vertexes map[reflect.Type]*Vertex
}

// AddEdge connects two vertices, in the direction
// of from -> to
func (g *Graph) AddEdge(from, to reflect.Type) {
	e := Edge{
		From: from,
		To:   to,
	}
	g.Edges = append(g.Edges, e)
	if vertex, has := g.Vertexes[from]; has {
		vertex.OutgoingEdges = append(vertex.OutgoingEdges, &e)
	}
	if vertex, has := g.Vertexes[to]; has {
		vertex.IncomingEdges = append(vertex.IncomingEdges, &e)
	}
}

type status int

const (
	statusTemporary status = iota
	statusPermanent
)

type vertexMarker map[reflect.Type]status

func (g Graph) checkCyclicDeps(l *Vertex, sorted *[]reflect.Type, marker vertexMarker) error {
	if status, has := marker[l.Typ]; has && status == statusPermanent {
		return nil
	}
	if status, has := marker[l.Typ]; has && status == statusTemporary {
		return fmt.Errorf("cyclic dependency on type: %v", l.Typ)
	}
	marker[l.Typ] = statusTemporary
	for _, e := range l.OutgoingEdges {
		if err := g.checkCyclicDeps(g.Vertexes[e.To], sorted, marker); err != nil {
			return fmt.Errorf("type %v error: %w", l.Typ, err)
		}
	}
	marker[l.Typ] = statusPermanent
	*sorted = append([]reflect.Type{l.Typ}, *sorted...)
	return nil
}

func (g Graph) Sort() ([]reflect.Type, error) {
	sortedVerticies := []reflect.Type{}
	marker := vertexMarker{}

	// quick sanity check on edges
	for _, e := range g.Edges {
		if _, has := g.Vertexes[e.From]; !has {
			return []reflect.Type{}, fmt.Errorf("no vertex found for type: %v: -> %v", e.From, e.To)
		}
	}

	for _, vertex := range g.Vertexes {
		if err := g.checkCyclicDeps(vertex, &sortedVerticies, marker); err != nil {
			return sortedVerticies, err
		}
	}
	return sortedVerticies, nil
}

var ErrNilFunction = errors.New("got nil, expected a function")

var inType = reflect.TypeOf(In{})

func isInType(typ reflect.Type) bool {
	if typ.Kind() != reflect.Struct {
		return false
	}
	fieldCount := typ.NumField()
	if fieldCount > 0 {
		for i := 0; i < fieldCount; i++ {
			field := typ.Field(i)
			if field.Type.AssignableTo(inType) {
				return true
			}
		}
	}
	return false
}

// GraphFromFunc takes in a function and returns a graph
// containing the functions dependencies and what types it returns
func GraphFromFunc(fn interface{}) (*Graph, error) {
	if fn == nil {
		return nil, ErrNilFunction
	}
	graph := NewGraph()
	fnVal := reflect.ValueOf(fn)
	fnTyp := fnVal.Type()
	if fnTyp.Kind() != reflect.Func {
		return nil, fmt.Errorf("expected a function, got: %v", fnTyp)
	}

	errLocation, err := getErrorLocation(fnTyp)
	if err != nil {
		return nil, err
	}

	type graphDep struct {
		reflect.Type
		IsInType bool
	}
	deps := []graphDep{}

	for i := 0; i < fnTyp.NumIn(); i++ {
		fnArgType := fnTyp.In(i)
		deps = append(deps,
			graphDep{
				Type:     fnArgType,
				IsInType: isInType(fnArgType),
			})
	}

	outCount := fnTyp.NumOut()
	for i := 0; i < outCount; i++ {
		out := fnTyp.Out(i)
		if _, has := graph.Vertexes[out]; !has {
			outgoingEdges := Edges{}
			incomingEdges := Edges{}

			// add any edges that already exist for this type
			for _, e := range graph.Edges {
				switch out {
				case e.From:
					outgoingEdges = append(outgoingEdges, &e)
				case e.To:
					incomingEdges = append(incomingEdges, &e)
				}
			}

			graph.Vertexes[out] = &Vertex{
				errorOutLocation: errLocation,
				OutgoingEdges:    outgoingEdges,
				IncomingEdges:    incomingEdges,
				Typ:              out,
				fn:               fnVal,
			}
		}
		for _, dep := range deps {
			out := fnTyp.Out(i)

			if dep == out {
				return nil, fmt.Errorf("cannot need and return the same type: %v", out)
			}

			graph.AddEdge(dep.Type, out)
			graph.Edges[len(graph.Edges)-1].hasSpecialInType = dep.IsInType
		}
	}

	if outCount <= 0 {
		for _, dep := range deps {
			graph.AddEdge(dep.Type, nil)
			graph.Edges[len(graph.Edges)-1].hasSpecialInType = dep.IsInType
		}
	}
	return graph, nil
}

func MergeGraphs(from, to *Graph) *Graph {
	// add all vertexes from 'from -> to'
	for t, v := range from.Vertexes {
		if _, has := to.Vertexes[t]; !has {
			to.Vertexes[t] = v
		}
	}

	// copy over all edges if they don't already exist
	for _, edge := range from.Edges {
		found := false
		for _, toEdge := range to.Edges {
			if edge.Equal(toEdge) {
				found = true
				break
			}
		}
		if !found {
			to.Edges = append(to.Edges, edge)
		}
	}
	return to
}
