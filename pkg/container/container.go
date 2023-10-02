package container

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/zhamlin/chi-openapi/internal"
	"github.com/zhamlin/chi-openapi/internal/graph"
	reflectUtil "github.com/zhamlin/chi-openapi/internal/reflect"
	runtimeUtil "github.com/zhamlin/chi-openapi/internal/runtime"
)

func MustCast[T any](obj any) any {
	v, ok := Cast[T](obj)
	if !ok {
		panic(fmt.Sprintf("%T could not be casted to %T", obj, *new(T)))
	}
	return v
}

func Cast[T any](obj any) (any, bool) {
	if t, ok := obj.(T); ok {
		isInterface := reflect.TypeOf(*new(T)) == nil
		if isInterface {
			typ := reflectUtil.MakeType[T]()
			v := reflect.New(typ)
			v.Elem().Set(reflect.ValueOf(t))
			return v.Interface(), true
		}
		return t, true
	}
	return obj, false
}

type Container = containerWithHooks

func New() Container {
	return newContainerWithHooks()
}

func newContainer() container {
	return container{
		graph:       graph.New[node](),
		typeIndexes: map[reflect.Type]int{},
	}
}

// Context contains the cached types from RunWithCtx or CreateWithCtx
type Context interface {
	Get(reflect.Type) (reflect.Value, bool)
	Run(fn any) error
}

func newContext(args ...any) context {
	ctx := context{
		cache: map[uintptr]reflect.Value{},
	}
	for i := 0; i < len(args); i++ {
		argValue := reflect.ValueOf(args[i])
		if argValue.Kind() == reflect.Ptr &&
			argValue.Elem().Kind() == reflect.Interface {
			// if a pointer to an interface if passed in assume it came
			// from container.Cast and get the value to the interface directly
			argValue = argValue.Elem()
		}
		ctx.set(argValue)
	}
	return ctx
}

type context struct {
	// uintptr represents the pointer to reflect.Value(typ).Pointer()
	// to speed up map access instead of using reflect.Type
	cache map[uintptr]reflect.Value
}

func (c context) Run(fn any) error {
	fnType := reflect.TypeOf(fn)
	fnValue := reflect.ValueOf(fn)
	fnParams := reflectUtil.GetFuncParams(fnType)
	_, err := runFn(fnValue, fnParams, func(t reflect.Type) (reflect.Value, error) {
		value, has := c.Get(t)
		if !has {
			return reflect.Value{}, fmt.Errorf("missing type: %s", t.String())
		}
		return value, nil
	})
	return err
}

func (c context) set(value reflect.Value) {
	// If the value is already set don't update it as it was
	// set from the args when creating the context. No value should
	// be created multiple times or have multiple sources in any other
	// scenario
	key := reflect.ValueOf(value.Type()).Pointer()
	if _, has := c.cache[key]; !has {
		c.cache[key] = value
	}
}

func (c context) Get(typ reflect.Type) (reflect.Value, bool) {
	key := reflect.ValueOf(typ).Pointer()
	value, has := c.cache[key]
	return value, has
}

type nodeType interface {
	node()
}

type nodeTypeValue struct{}

func (nodeTypeValue) node() {}

type nodeTypeProvider struct {
	value  reflect.Value
	params []reflect.Type
}

func (n nodeTypeProvider) isFunc() bool {
	return n.value.Kind() == reflect.Func
}

func (nodeTypeProvider) node() {}

func newProviderNodeFromObj(v reflect.Value) node {
	return node{
		typ: nodeTypeProvider{
			value: v,
		},
		refelctType: v.Type(),
	}
}

func newProviderNodeFromFn(fn reflect.Value) (node, error) {
	typ := fn.Type()
	if fn.Kind() != reflect.Func {
		return node{}, fmt.Errorf("expected func got: %s", typ.String())
	}
	fnParams := reflectUtil.GetFuncParams(typ)
	return node{
		typ: nodeTypeProvider{
			value:  fn,
			params: fnParams,
		},
		refelctType: typ,
	}, nil
}

func newNodeFromType(typ reflect.Type) node {
	return node{
		typ:         nodeTypeValue{},
		refelctType: typ,
	}
}

type node struct {
	typ         nodeType
	refelctType reflect.Type
}

func (n node) Type() reflect.Type {
	return n.refelctType
}

// graph methods
func (n node) String() string {
	if n.refelctType == reflectUtil.ErrType {
		return ""
	}
	return n.refelctType.String()
}

func (n node) NodeShape() string {
	switch n.typ.(type) {
	case nodeTypeProvider:
		return "box"
	case nodeTypeValue:
		return "circle"
	default:
		panic("unknown node type")
	}
}

type container struct {
	graph *graph.Graph[node]
	// type to its provider node index
	typeIndexes map[reflect.Type]int
}

func (c container) CheckForCycles() error {
	_, err := graph.TopologicalSort(c.graph)
	return err
}

func (c container) handleErr(err error) {
	if caller := runtimeUtil.GetCaller(2); caller != "" {
		err = fmt.Errorf("%s: %w", caller, err)
	}
	panic(err)
}

func (c container) HasType(typ reflect.Type) bool {
	_, has := c.typeIndexes[typ]
	return has
}

func (c container) Provide(obj any) {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Func {
		c.provideFn(v)
	} else {
		c.provideObj(v)
	}
}

func (c container) provideObj(obj reflect.Value) {
	node := newProviderNodeFromObj(obj)
	providerIdx := c.graph.Add(node)

	// add non provider node for this type
	t := obj.Type()
	node = newNodeFromType(t)
	idx := c.graph.Add(node)
	if err := c.graph.AddEdges(providerIdx, idx); err != nil {
		c.handleErr(err)
	}
	c.typeIndexes[t] = idx
}

// provideFn will add the fn to the containers graph as a provider node
func (c container) provideFn(fn reflect.Value) {
	fnNode, err := newProviderNodeFromFn(fn)
	if err != nil {
		c.handleErr(err)
		return
	}

	fnType := fnNode.refelctType
	fnOutput := reflectUtil.GetFuncOutputs(fnType)

	errLoc, hasErr := reflectUtil.GetErrorLocation(fnType)
	errIsLastOutput := errLoc == len(fnOutput)-1
	if hasErr && !errIsLastOutput {
		err := fmt.Errorf("error must be the last output fn: %s", fnType.String())
		c.handleErr(err)
		return
	}

	fnNodeID := c.graph.Add(fnNode)

	// add output edges from this func
	for _, output := range fnOutput {
		nodeIdx, has := c.typeIndexes[output]
		if !has {
			nodeIdx = c.graph.Add(newNodeFromType(output))
			c.typeIndexes[output] = nodeIdx
		}
		if err := c.graph.AddEdges(fnNodeID, nodeIdx); err != nil {
			c.handleErr(err)
			return
		}
	}

	fnParams := reflectUtil.GetFuncParams(fnType)
	// add input edges to this func from params
	for _, param := range fnParams {
		nodeIdx, has := c.typeIndexes[param]
		if !has {
			nodeIdx = c.graph.Add(newNodeFromType(param))
			c.typeIndexes[param] = nodeIdx
		}
		if err := c.graph.AddEdges(nodeIdx, fnNodeID); err != nil {
			c.handleErr(err)
			return
		}
	}
}

func (c container) createWithCtx(ctx context, obj any) error {
	objType := reflect.TypeOf(obj)
	if objType.Kind() != reflect.Ptr {
		return fmt.Errorf("got: %T expected reference to type", obj)
	}

	value, err := c.createType(ctx, objType.Elem())
	if err != nil {
		return err
	}

	objValue := reflect.ValueOf(obj)
	if objValue.CanSet() {
		objValue.Set(value)
	} else if elem := objValue.Elem(); elem.CanSet() {
		elem.Set(value)
	}
	return nil
}

func (c container) Create(obj any, args ...any) error {
	ctx := newContext(args...)
	return c.createWithCtx(ctx, obj)
}

// See RunWithCtx
func (c container) Run(fn any, args ...any) (any, error) {
	return c.runWithCtx(newContext(args...), fn)
}

// IsValidRunFunc checks to see if the provided type
// matches the expected function signature for container.Run methods.
// The function must be one of the following:
// - func(...)
// - func(...) [T | error]
// - func(...) (T, error)
func IsValidRunFunc(t reflect.Type) error {
	if kind := t.Kind(); kind != reflect.Func {
		return fmt.Errorf("expected func got: %v", kind)
	}

	outputs := reflectUtil.GetFuncOutputs(t)
	l := len(outputs)
	switch l {
	case 0:
	case 1:
	case 2:
		if outputs[1] != reflectUtil.ErrType {
			return fmt.Errorf("last output must be an error not: %s", outputs[1].String())
		}
	default:
		return fmt.Errorf("more than two outputs in func: %d", l)
	}
	return nil
}

func getRespAndError(outputs []reflect.Value) (any, error) {
	l := len(outputs)
	switch l {
	case 0:
		return nil, nil
	case 1:
		// any|error
		output := outputs[0]
		if err := isNonNilErr(output); err != nil {
			return nil, err
		}
		return output.Interface(), nil
	case 2:
		// (any, error)
		if err := isNonNilErr(outputs[1]); err != nil {
			return nil, err
		}
		return outputs[0].Interface(), nil
	}
	return nil, fmt.Errorf("more than two outputs in func: %d", l)
}

type Plan struct {
	providers []*nodeTypeProvider
	fn        reflect.Value
	fnParams  []reflect.Type

	// Track the max array size needed to contain all of the functions params.
	// This allows one allocation for all of the function inputs once per plan run
	maxParamCount int
}

// CreatePlan walks the fn inputs ensuring it has all the needed
// types to call the fn. The returned plan can be used to call the fn
// via RunPlan, which is faster than calling Run if you plan on
// calling the function multiple times. See IsValidRunFunc for
// more information regarding fn.
func (c container) CreatePlan(fn any, ignore ...any) (Plan, error) {
	fnType := reflect.TypeOf(fn)
	if err := IsValidRunFunc(fnType); err != nil {
		return Plan{}, err
	}

	fnParams := reflectUtil.GetFuncParams(fnType)
	plan := Plan{
		providers:     []*nodeTypeProvider{},
		fn:            reflect.ValueOf(fn),
		fnParams:      fnParams,
		maxParamCount: len(fnParams),
	}

	neededTypes := internal.NewSet[reflect.Type]()
	// from the provided index recursively get the indexes of all
	// the provider nodes required to create it.
	var visit func(idx int) []*nodeTypeProvider
	visit = func(idx int) []*nodeTypeProvider {
		result := []*nodeTypeProvider{}
		for edgeIndex := range c.graph.EdgesToFrom[idx] {
			n, err := c.graph.Get(edgeIndex)
			if err != nil {
				// if the index stored in the graph is invalid something
				// has gone wrong
				panic(err)
			}

			if provider, ok := n.typ.(nodeTypeProvider); ok {
				if provider.isFunc() {
					if c := len(provider.params); c > plan.maxParamCount {
						plan.maxParamCount = c
					}
				}
				result = append(result, &provider)
			} else {
				neededTypes.Add(n.refelctType)
			}
			result = append(result, visit(edgeIndex)...)
		}
		return result
	}

	shouldIgnore := func(t reflect.Type) bool {
		for _, item := range ignore {
			if itemType, ok := item.(reflect.Type); ok {
				if itemType == t {
					return true
				}
			}
			if reflect.TypeOf(item) == t {
				return true
			}
		}
		return false
	}

	canCreateType := func(t reflect.Type, idx int, has bool) bool {
		missingTypeProvider := len(c.graph.EdgesFromTo[idx]) > 0 && len(c.graph.EdgesToFrom[idx]) < 1
		missingType := missingTypeProvider || !has
		return !missingType || shouldIgnore(t)
	}

	for _, pType := range plan.fnParams {
		idx, has := c.typeIndexes[pType]
		if !canCreateType(pType, idx, has) {
			return plan, fmt.Errorf("container can not create the type: %s", pType.String())
		}
		plan.providers = append(plan.providers, visit(idx)...)
	}

	// verify the needed types can be created
	for t := range neededTypes {
		idx, has := c.typeIndexes[t]
		if !canCreateType(t, idx, has) {
			return plan, fmt.Errorf("container can not create the type: %s", t.String())
		}
		if len(c.graph.EdgesFromTo[idx]) > 0 && len(c.graph.EdgesToFrom[idx]) > 1 {
			return plan, fmt.Errorf("container has more than one way to create the type: %s", t.String())
		}
	}

	// reverse array so nodes with no deps come first
	internal.Reverse(plan.providers)

	// remove duplicates to avoid extra work
	plan.providers = internal.Unique(plan.providers)
	return plan, nil
}

func (c container) RunPlan(plan Plan, args ...any) (any, error) {
	return c.runPlanWithContext(newContext(args...), plan)
}

var ErrInvalidPlan = errors.New("incorrect plan provided")

func (c container) runPlanWithContext(ctx context, plan Plan) (any, error) {
	if !plan.fn.IsValid() {
		return nil, ErrInvalidPlan
	}

	inputs := make([]reflect.Value, plan.maxParamCount)
	callFn := func(fn reflect.Value, params []reflect.Type) ([]reflect.Value, error) {
		for i, pType := range params {
			param, has := ctx.Get(pType)
			if !has {
				// this _shouldn't_ happen unless this type was ignored during
				// plan creation
				return nil, fmt.Errorf("context did not have %s", pType.String())
			}
			inputs[i] = param
		}
		return fn.Call(inputs[:len(params)]), nil
	}

	for _, provider := range plan.providers {
		output := []reflect.Value{provider.value}
		if provider.isFunc() {
			var err error
			output, err = callFn(provider.value, provider.params)
			if err != nil {
				return nil, err
			}
		}

		for _, o := range output {
			if err := isNonNilErr(o); err != nil {
				return nil, err
			}
			ctx.set(o)
		}
	}

	output, err := callFn(plan.fn, plan.fnParams)
	if err != nil {
		return nil, err
	}
	return getRespAndError(output)
}

// RunWithCtx runs the provided fn creating the parameters as needed.
// If a parameter can not be created an error is return. Upon success the
// response and error from the func are returned. See IsValidRunFunc for
// more information.
func (c container) runWithCtx(ctx context, fn any) (any, error) {
	fnType := reflect.TypeOf(fn)
	if err := IsValidRunFunc(fnType); err != nil {
		return Plan{}, err
	}
	fnValue := reflect.ValueOf(fn)
	fnParams := reflectUtil.GetFuncParams(fnType)

	outputs, err := c.runFn(ctx, fnType, fnValue, fnParams)
	if err != nil {
		return nil, err
	}
	return getRespAndError(outputs)
}

func isNonNilErr(v reflect.Value) error {
	if v.Kind() != reflect.Interface {
		return nil
	}
	if v.IsNil() {
		return nil
	}
	isErrType := v.Type() == reflectUtil.ErrType
	if isErrType {
		// panic if this fails, check above should ensure
		// that does not happen
		outErr := v.Interface().(error)
		return outErr
	}
	return nil
}

type typeLoader func(reflect.Type) (reflect.Value, error)

func runFn(
	fnValue reflect.Value,
	fnParams []reflect.Type,
	loader typeLoader,
) ([]reflect.Value, error) {
	inputParams := make([]reflect.Value, 0, len(fnParams))
	for _, param := range fnParams {
		value, err := loader(param)
		if err != nil {
			return nil, err
		}
		inputParams = append(inputParams, value)
	}
	outputs := fnValue.Call(inputParams)
	for _, output := range outputs {
		if err := isNonNilErr(output); err != nil {
			return nil, err
		}
	}
	return outputs, nil
}

func (c container) runFn(
	ctx context,
	fnType reflect.Type,
	fnValue reflect.Value,
	fnParams []reflect.Type,
) ([]reflect.Value, error) {
	return runFn(fnValue, fnParams, func(t reflect.Type) (reflect.Value, error) {
		return c.createType(ctx, t)
	})
}

func (c container) createType(ctx context, typ reflect.Type) (reflect.Value, error) {
	if value, has := ctx.Get(typ); has {
		return value, nil
	}

	var emptyValue = reflect.Value{}
	typIndex, has := c.typeIndexes[typ]
	if !has {
		return emptyValue, fmt.Errorf("unsupported type: %v", typ.String())
	}

	edges := c.graph.EdgesToFrom
	indexes, has := edges[typIndex]
	if !has || len(indexes) == 0 {
		return emptyValue, fmt.Errorf("container can not create the type: %v", typ.String())
	}
	if len(indexes) > 1 {
		return emptyValue, fmt.Errorf("more than one way to create type: %v", typ.String())
	}

	nodeIndex := indexes.Items()[0]
	providerNode, err := c.graph.Get(nodeIndex)
	if err != nil {
		return emptyValue, err
	}

	switch nodeType := providerNode.typ.(type) {
	case nodeTypeProvider:
		if nodeType.isFunc() {
			outputs, err := c.runFn(ctx, providerNode.refelctType, nodeType.value, nodeType.params)
			if err != nil {
				return emptyValue, err
			}
			for _, value := range outputs {
				ctx.set(value)
			}
			value, has := ctx.Get(typ)
			if !has {
				panic("function did not return the expected type")
			}
			return value, nil
		}
		return nodeType.value, nil
	case nodeTypeValue:
		panic("non provider node can not create types")
	default:
		panic(fmt.Sprintf("invalid node type: %T", nodeType))
	}
}

func (c container) DotGraph() string {
	return graph.GraphToDot(c.graph)
}
