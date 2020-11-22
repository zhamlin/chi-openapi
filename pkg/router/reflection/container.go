package reflection

import (
	"fmt"
	"reflect"
)

type loader struct {
	fn               reflect.Value
	errorOutLocation int

	// slice of pointers to the graphs edges
	// relating to this vertex
	outgoingEdges []*edge
	incomingEdges []*edge
	typ           reflect.Type
}

type edge struct {
	From reflect.Type
	To   reflect.Type
}

type graph struct {
	Edges     []edge
	Verticies map[reflect.Type]*loader
}

func (g *graph) AddEdge(from, to reflect.Type) {
	e := edge{
		From: from,
		To:   to,
	}
	g.Edges = append(g.Edges, e)
	if vertex, has := g.Verticies[from]; has {
		vertex.outgoingEdges = append(vertex.outgoingEdges, &e)
	}
	if vertex, has := g.Verticies[to]; has {
		vertex.incomingEdges = append(vertex.incomingEdges, &e)
	}
}

type vertexStatus int

const (
	vStatusUnmarked vertexStatus = iota
	vStatusTemporary
	vStatusPermanent
)

type vertexMarker map[reflect.Type]vertexStatus

func (g graph) checkCyclicDepsUtil(l *loader, sorted *[]reflect.Type, marker vertexMarker) error {
	if status, has := marker[l.typ]; has && status == vStatusPermanent {
		return nil
	}
	if status, has := marker[l.typ]; has && status == vStatusTemporary {
		return fmt.Errorf("cyclic dep on type: %v", l.typ)
	}
	marker[l.typ] = vStatusTemporary
	for _, e := range l.outgoingEdges {
		if err := g.checkCyclicDepsUtil(g.Verticies[e.To], sorted, marker); err != nil {
			return err
		}
	}
	marker[l.typ] = vStatusPermanent
	*sorted = append([]reflect.Type{l.typ}, *sorted...)
	return nil
}

func (g graph) Sort() ([]reflect.Type, error) {
	sortedVerticies := []reflect.Type{}
	marker := vertexMarker{}

	// quick sanity check on edges
	for _, e := range g.Edges {
		if _, has := g.Verticies[e.From]; !has {
			return []reflect.Type{}, fmt.Errorf("no vertex found for type: %v", e.From)
		}
	}

	for _, vertex := range g.Verticies {
		if err := g.checkCyclicDepsUtil(vertex, &sortedVerticies, marker); err != nil {
			return sortedVerticies, err
		}
	}
	return sortedVerticies, nil
}

func NewContainer() *container {
	return &container{
		Graph: graph{
			Edges:     []edge{},
			Verticies: map[reflect.Type]*loader{},
		},
	}
}

type container struct {
	Graph graph
}

func (c *container) Provide(fn interface{}) error {
	typ := reflect.TypeOf(fn)
	if typ.Kind() != reflect.Func {
		return fmt.Errorf("expected a function, got: %v", typ)
	}
	deps := []reflect.Type{}
	for i := 0; i < typ.NumIn(); i++ {
		deps = append(deps, typ.In(i))
	}

	provides := []reflect.Type{}
	errCount := 0
	errLocation := -1
	for i := 0; i < typ.NumOut(); i++ {
		out := typ.Out(i)
		if out.Implements(errType) {
			errCount++
			errLocation = i
			if errCount > 1 {
				return fmt.Errorf("function cannot return more than one error")
			}
		}
	}

	for i := 0; i < typ.NumOut(); i++ {
		out := typ.Out(i)
		if _, has := c.Graph.Verticies[out]; !has {
			outgoingEdges := []*edge{}
			incomingEdges := []*edge{}
			// add any edges that already exist for this type
			for _, e := range c.Graph.Edges {
				if e.From == out {
					outgoingEdges = append(outgoingEdges, &e)
				}
				if e.To == out {
					incomingEdges = append(incomingEdges, &e)
				}
			}

			c.Graph.Verticies[out] = &loader{
				errorOutLocation: errLocation,
				outgoingEdges:    outgoingEdges,
				incomingEdges:    incomingEdges,
				typ:              out,
				fn:               reflect.ValueOf(fn),
			}
		}

		for _, dep := range deps {
			if dep == out {
				return fmt.Errorf("cannot need and return the same type: %v", out)
			}
			c.Graph.AddEdge(dep, out)
		}
		provides = append(provides)
	}
	return nil
}

// Execute will try and call the function with all of the arguments.
// Will need a value to container the already created values,
// this will prevent one func being called multiple times.
// Probably should have that passed directly to this, and maybe even returned.
// Would allow the caller to choose whether or not they want a new value
func (c container) Execute(fn interface{}, args ...interface{}) (interface{}, error) {
	val := reflect.ValueOf(fn)
	typ := val.Type()
	errLocation := -1
	errCount := 0
	for i := 0; i < typ.NumOut(); i++ {
		out := typ.Out(i)
		if out.Implements(errType) {
			errCount++
			errLocation = i
			if errCount > 1 {
				return nil, fmt.Errorf("function cannot return more than one error")
			}
		}
	}
	return c.execute(val, errLocation, args...)
}

// findError removes any empty errors if any, or returns an error if found
// expects an errLocation so it knows where to check
func findError(errLoc int, values []reflect.Value) ([]reflect.Value, error) {
	if errLoc < 0 {
		return values, nil
	}

	errValue := values[errLoc]
	if errValue.IsZero() || errValue.IsNil() {
		// remove the error from the return array
		values = append(values[:errLoc], values[errLoc+1:]...)
	} else {
		if errValue.Type().Implements(errType) {
			e := errValue.Elem().Interface().(error)
			values = append(values[:errLoc], values[errLoc+1:]...)
			return values, e
		}
		return values, fmt.Errorf("expected an error type value for the %v return type", errLoc)
	}
	return values, nil
}

func (c container) execute(fn reflect.Value, errLocation int, args ...interface{}) ([]reflect.Value, error) {
	typ := fn.Type()
	if typ.Kind() != reflect.Func {
		return nil, fmt.Errorf("expected a function, got: %v", typ)
	}
	vals := []reflect.Value{}
	if typ.NumIn() == 0 {
		results := fn.Call([]reflect.Value{})
		// swap return values with passed in value, if any
		for _, arg := range args {
			for i := 0; i < len(results); i++ {
				if results[i].Type() == reflect.TypeOf(arg) {
					results[i] = reflect.ValueOf(arg)
					break
				}
			}
		}
		return findError(errLocation, results)
	}

	for i := 0; i < typ.NumIn(); i++ {
		createArgs := []reflect.Value{}
		t := typ.In(i)

		// whether or not this type was explicitly passed to execute
		providedType := false
		for _, arg := range args {
			if t == reflect.TypeOf(arg) {
				createArgs = append(createArgs, reflect.ValueOf(arg))
				vals = append(vals, reflect.ValueOf(arg))
				providedType = true
				break
			}
		}

		if providedType {
			continue
		}

		l, has := c.Graph.Verticies[t]
		if !has {
			return vals, fmt.Errorf("don't know how to create type: %v", t)
		}
		if !l.fn.IsValid() {
			return vals, fmt.Errorf("create function is nil for %v", t)
		}

		// walk down the dependency tree, and create each type
		for _, edge := range l.incomingEdges {
			if val, has := c.Graph.Verticies[edge.From]; has {
				results, err := c.execute(val.fn, val.errorOutLocation, args...)
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
		vals = append(vals, results...)

		// TODO: add these values to a cache of some kind
	}
	results := fn.Call(vals)
	return findError(errLocation, results)
}
