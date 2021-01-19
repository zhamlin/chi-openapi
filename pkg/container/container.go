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
			return []reflect.Type{}, fmt.Errorf("no vertex found for type: %v", e.From)
		}
	}

	for _, vertex := range g.Vertexes {
		if err := g.checkCyclicDeps(vertex, &sortedVerticies, marker); err != nil {
			return sortedVerticies, err
		}
	}
	return sortedVerticies, nil
}

func NewContainer() *Container {
	return &Container{
		Graph: NewGraph(),
	}
}

type Container struct {
	Graph *Graph
}

func (c *Container) HasType(t reflect.Type) bool {
	_, has := c.Graph.Vertexes[t]
	return has
}

var errType = reflect.TypeOf((*error)(nil)).Elem()

// getErrorLocation validates that the function is returning at most one error
// and returns the location if it is returning an error.
func getErrorLocation(fnType reflect.Type) (int, error) {
	errCount := 0
	errLocation := -1

	for i := 0; i < fnType.NumOut(); i++ {
		out := fnType.Out(i)
		if out.Implements(errType) {
			errCount++
			errLocation = i
			if errCount > 1 {
				return -1, errors.New("function cannot return more than one error")
			}
		}
	}
	return errLocation, nil
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

var ErrNilFunction = errors.New("got nil, expected a function")

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

	deps := []reflect.Type{}
	for i := 0; i < fnTyp.NumIn(); i++ {
		deps = append(deps, fnTyp.In(i))
	}
	for i := 0; i < fnTyp.NumOut(); i++ {
		out := fnTyp.Out(i)
		if _, has := graph.Vertexes[out]; !has {
			outgoingEdges := []*Edge{}
			incomingEdges := []*Edge{}

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
			if dep == out {
				return nil, fmt.Errorf("cannot need and return the same type: %v", out)
			}
			graph.AddEdge(dep, out)
		}
	}
	return graph, nil
}

// Provide adds the return types of the function as
// vertexes on a graph and attempts to add edges
// based on the arguments of the function
func (c *Container) Provide(fn interface{}) error {
	newGraph, err := GraphFromFunc(fn)
	if err != nil {
		return fmt.Errorf("getting a graph from the function: %w", err)
	}
	c.Graph = MergeGraphs(newGraph, c.Graph)
	return nil
}

// Execute will try and call the function with all of the arguments.
func (c Container) Execute(fn interface{}, args ...interface{}) (interface{}, error) {
	val := reflect.ValueOf(fn)
	typ := val.Type()

	errLocation, err := getErrorLocation(typ)
	if err != nil {
		return nil, err
	}

	// we can't just throw args in the cache because
	// a function might expect an interface, and we only have the exact
	// types for the args, not interfaces
	cache := map[reflect.Type]reflect.Value{}
	values, err := c.execute(val, errLocation, cache, args...)
	if err != nil {
		return nil, err
	}
	if l := len(values); l > 0 {
		return values[0].Interface(), nil
	}
	return nil, nil
}

// findError will return the error if it is non nil
// if not it wil remove the empty error from the values slice
func findError(errLoc int, values []reflect.Value) ([]reflect.Value, error) {
	if errLoc < 0 {
		return values, nil
	}
	if l := len(values); errLoc > l {
		return nil, fmt.Errorf("error location is index %v, slice length is %v", errLoc, l)
	}

	errValue := values[errLoc]
	if errValue.IsZero() || errValue.IsNil() {
		// remove the error from the return array
		values = append(values[:errLoc], values[errLoc+1:]...)
		return values, nil
	}
	if errValue.Type().Implements(errType) {
		e := errValue.Elem().Interface().(error)
		values = append(values[:errLoc], values[errLoc+1:]...)
		return values, e
	}
	return values, fmt.Errorf("expected an error type value for the %v return type, got %v", errLoc, errValue.Type())
}

func (c Container) execute(fn reflect.Value, errLocation int, cache map[reflect.Type]reflect.Value, args ...interface{}) ([]reflect.Value, error) {
	typ := fn.Type()
	if typ.Kind() != reflect.Func {
		return nil, fmt.Errorf("expected a function, got: %v", typ)
	}

	if typ.NumIn() == 0 {
		results := fn.Call([]reflect.Value{})
		// swap return values with passed in value if any
		// if this function returns errors, ignore them because
		// we are directly inserting that value
		for _, arg := range args {
			argType := reflect.TypeOf(arg)
			for i := 0; i < len(results); i++ {
				rType := results[i].Type()
				// fmt.Printf("%v | %v %v\n", typ, rType, argType)
				if argType.AssignableTo(rType) {
					results[i] = reflect.ValueOf(arg)
					// remove the error, we aren't using this function
					if errLocation > -1 {
						results = append(results[:errLocation], results[errLocation+1:]...)
					}
					return results, nil
				}
			}
		}
		return findError(errLocation, results)
	}

	vals := []reflect.Value{}
	for i := 0; i < typ.NumIn(); i++ {
		createArgs := []reflect.Value{}
		t := typ.In(i)

		// if in cache, use that value instead to prevent
		// expensive functions from being called twice
		if cached, has := cache[t]; has {
			vals = append(vals, cached)
			continue
		}

		// whether or not this type was explicitly passed to execute
		providedType := false
		for _, arg := range args {
			argType := reflect.TypeOf(arg)
			if argType.AssignableTo(t) {
				vals = append(vals, reflect.ValueOf(arg))
				providedType = true
				break
			}
		}
		if providedType {
			continue
		}

		l, has := c.Graph.Vertexes[t]
		if !has {
			return vals, fmt.Errorf("don't know how to create type: %v", t)
		}
		if !l.fn.IsValid() {
			return vals, fmt.Errorf("create function is nil for %v", t)
		}

		// walk down the dependency tree, and create each type
		for _, edge := range l.IncomingEdges {
			if val, has := c.Graph.Vertexes[edge.From]; has {
				results, err := c.execute(val.fn, val.errorOutLocation, cache, args...)
				if err != nil {
					return vals, err
				}
				createArgs = append(createArgs, results...)
			}
		}
		results := l.fn.Call(createArgs)
		results, err := findError(l.errorOutLocation, results)
		if err != nil {
			return vals, err
		}

		// add every type that wasn't an error returned to the cache
		for _, result := range results {
			rType := result.Type()
			cache[rType] = result

			// this is our current argument, add it to the vals in the correct order here
			if t.AssignableTo(rType) {
				vals = append(vals, result)
			}
		}
	}
	results := fn.Call(vals)
	return findError(errLocation, results)
}

// CreateType returns a newly created value of the supplied type
func (c Container) CreateType(typ reflect.Type, args ...interface{}) (interface{}, error) {
	dynamicFuncType := reflect.FuncOf([]reflect.Type{typ}, []reflect.Type{typ}, false)
	dynamicFunc := func(in []reflect.Value) []reflect.Value {
		return []reflect.Value{in[0]}
	}
	fn := reflect.MakeFunc(dynamicFuncType, dynamicFunc)
	return c.Execute(fn.Interface(), args...)
}
