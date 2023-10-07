package container

import (
	"errors"
	"fmt"
	"reflect"

	reflectUtil "github.com/zhamlin/chi-openapi/internal/reflect"
)

type hook func(Context, error) error

func newContainerWithHooks() containerWithHooks {
	return containerWithHooks{
		container: newContainer(),
		hooks:     map[reflect.Type]hook{},
	}
}

type containerWithHooks struct {
	container
	hooks map[reflect.Type]hook
}

func (c containerWithHooks) Run(fn any, args ...any) (any, error) {
	ctx := newContext(args...)
	res, err := c.runWithCtx(ctx, fn)
	if len(c.hooks) == 0 {
		return res, err
	}
	return res, runHooks(ctx, c.hooks, err)
}

func (c containerWithHooks) RunPlan(plan Plan, args ...any) (any, error) {
	ctx := newContext(args...)
	res, err := c.runPlanWithContext(ctx, plan)
	if len(c.hooks) == 0 {
		return res, err
	}
	return res, runHooks(ctx, c.hooks, err)
}

func (c containerWithHooks) Create(obj any, args ...any) error {
	ctx := newContext(args...)
	err := c.createWithCtx(ctx, obj)
	if len(c.hooks) == 0 {
		return err
	}
	return runHooks(ctx, c.hooks, err)
}

var ErrHookError = errors.New("container hook returned err")
var ErrHookCausedError = errors.New("container hook caused the error")

func runHooks(ctx Context, hooks map[reflect.Type]hook, err error) error {
	errs := []error{}
	if err != nil {
		errs = append(errs, err)
	}
	for _, hook := range hooks {
		if hookErr := hook(ctx, err); hookErr != nil {
			errs = append(errs, fmt.Errorf("%w: %w", ErrHookError, hookErr))
		}
	}
	startedWithNoErr := err == nil
	haveErrors := len(errs) > 0
	if startedWithNoErr && haveErrors {
		errs = append(errs, ErrHookCausedError)
	}
	return errors.Join(errs...)
}

func get[T any](ctx Context) (T, bool) {
	typ := reflectUtil.MakeType[T]()
	value, has := ctx.Get(typ)
	if !has {
		return *new(T), false
	}
	obj, ok := value.Interface().(T)
	return obj, ok
}

// After registers the fn to run after any call to CreateWithCtx or
// RunWithCtx where the provided type was created. If an error is returned
// it will cause the original function to fail with that error.
func After[T any](c Container, fn func(Context, T, error) error) {
	typ := reflect.TypeOf(new(T))
	c.hooks[typ] = func(ctx Context, err error) error {
		if obj, has := get[T](ctx); has {
			return fn(ctx, obj, err)
		}
		return nil
	}
}
