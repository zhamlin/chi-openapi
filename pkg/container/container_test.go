package container_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	reflectUtil "github.com/zhamlin/chi-openapi/internal/reflect"
	. "github.com/zhamlin/chi-openapi/internal/testing"
	"github.com/zhamlin/chi-openapi/pkg/container"
)

func TestIsValidRunFunc(t *testing.T) {
	tests := []struct {
		name    string
		fn      any
		wantErr bool
	}{
		{
			fn: func() {},
		},
		{
			fn: func() int {
				return 1
			},
		},
		{
			fn: func() error {
				return nil
			},
		},
		{
			fn: func() (int, error) {
				return 1, nil
			},
		},
		{
			name: "func has too many outputs",
			fn: func() (int, int, int) {
				return 1, 2, 3
			},
			wantErr: true,
		},
		{
			name: "last output must be error",
			fn: func() (int, int) {
				return 1, 2
			},
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := container.IsValidRunFunc(reflect.TypeOf(test.fn))
			if test.wantErr {
				MustNotMatch(t, err, nil, "expected an error got none")
			} else {
				MustMatch(t, err, nil, "did not expect an error")
			}
		})
	}
}

func TestContainer(t *testing.T) {
	t.Run("show provided args work", func(t *testing.T) {
		expectedString := "hello world"

		c := container.New()
		c.Provide(func() string {
			return expectedString
		})

		var value string
		err := c.Create(&value)
		MustMatch(t, err, nil)
		MustMatch(t, value, expectedString)
	})

	t.Run("show values are cached", func(t *testing.T) {
		c := container.New()

		calls := 0
		c.Provide(func() string {
			calls += 1
			return "string"
		})

		type A struct{}
		c.Provide(func(string) A {
			return A{}
		})
		type B struct{}
		c.Provide(func(string) (B, bool, int) {
			return B{}, true, 0
		})
		type C struct {
			B bool
		}
		c.Provide(func(_ string, b bool) C {
			return C{B: b}
		})
		var cObj C
		err := c.Create(&cObj)
		MustMatch(t, cObj.B, true)
		MustMatch(t, err, nil)
		MustMatch(t, calls, 1, "expected the string type to be cached")
	})

	t.Run("provide non fn types", func(t *testing.T) {
		expectedString := "foobar"
		c := container.New()
		c.Provide(expectedString)

		var value string
		err := c.Create(&value)
		MustMatch(t, err, nil)
		MustMatch(t, value, expectedString)

		plan, err := c.CreatePlan(func(s string) string {
			return s
		})
		MustMatch(t, err, nil)
		resp, err := c.RunPlan(plan)
		MustMatch(t, err, nil, "expected no error creating plan")
		respStr := resp.(string)
		MustMatch(t, respStr, expectedString)
	})
}

func TestContainerMisc(t *testing.T) {
	type A struct{}
	a := func() A {
		return A{}
	}

	type B struct{}
	b := func() B {
		return B{}
	}

	type C struct{}
	cFn := func(a A, b B) C {
		return C{}
	}

	type D struct {
		Name string
	}

	d := func(c C) D {
		return D{
			Name: "From C",
		}
	}

	c := container.New()

	c.Provide(a)
	c.Provide(b)
	c.Provide(cFn)
	c.Provide(d)

	MustMatch(t, c.CheckForCycles(), nil, "CheckForCycles failed")
	// TODO
	// dObj, err := container.Create[D](c)
	// MustMatch(t, err, nil)
	// MustMatch(t, dObj.Name, "From C")
}

func TestContainerHooks(t *testing.T) {
	c := container.New()
	type A struct {
		A string
	}
	type B struct {
		B string
	}

	afterARan := false
	container.After(c, func(ctx container.Context, a A, err error) error {
		afterARan = true
		return errors.New("error from A")
	})

	afterBRan := false
	container.After(c, func(ctx container.Context, b B, err error) error {
		MustMatch(t, b.B, "A")
		afterBRan = true
		return errors.New("error from B")
	})

	c.Provide(func() A { return A{"A"} })
	c.Provide(func(a A) B { return B{B: a.A} })

	t.Run("all hooks run", func(t *testing.T) {
		var b B
		err := c.Create(&b)
		MustMatch(t, errors.Is(err, container.ErrHookError), true, "expected ErrHookFailed")
		MustMatch(t, errors.Is(err, container.ErrHookCausedError), true, "expected ErrHookCausedFailure")
		MustMatch(t, b.B, "A", "B was set because only the hooks failed")
		errs, ok := err.(interface{ Unwrap() []error })
		MustMatch(t, ok, true, "error should implement Unwrap() []error")
		// one error from each hook and one for ErrHookCausedFailure
		MustMatch(t, len(errs.Unwrap()), 3)
		MustMatch(t, afterARan, true, "A after hook did not run")
		MustMatch(t, afterBRan, true, "B after hook did not run")
	})

	t.Run("hooks run when fn errors", func(t *testing.T) {
		_, err := c.Run(func(b B) error {
			return errors.New("error")
		})
		MustMatch(t, errors.Is(err, container.ErrHookError), true, "expected ErrHookFailed")
		MustMatch(t, errors.Is(err, container.ErrHookCausedError), false, "expected no ErrHookCausedFailure")
	})

	t.Run("hooks only run when type is created", func(t *testing.T) {
		afterBRan = false
		afterARan = false

		_, err := c.Run(func(A) error { return nil })
		MustNotMatch(t, err, nil)
		MustMatch(t, afterARan, true)
		MustMatch(t, afterBRan, false)
	})
}

func TestContainerPlan(t *testing.T) {
	type A struct {
		N int
	}
	aFn := func() A {
		return A{N: 1}
	}
	type B struct {
		N int
	}
	bFn := func(a A) B {
		return B{N: a.N * 2}
	}
	type C struct {
		N int
	}
	cFn := func(b B) C {
		return C{N: b.N * 3}
	}
	type D struct {
		N int
	}
	dFn := func(c C) (D, error) {
		if c.N < 0 {
			return D{}, errors.New("D error")
		}
		return D{N: c.N * 4}, nil
	}

	c := container.New()
	c.Provide(aFn)
	c.Provide(bFn)
	c.Provide(cFn)
	c.Provide(dFn)

	type runnerFn func(fn any, args ...any) (any, error)

	test := func(t *testing.T, fn any, runner runnerFn) {
		resp, err := runner(fn)
		MustMatch(t, err, nil)
		d := resp.(D)
		MustMatch(t, d.N, 24)

		resp, err = runner(fn, C{N: 0})
		d = resp.(D)
		MustMatch(t, err, nil)
		MustMatch(t, d.N, 0)

		_, err = runner(fn, C{N: -1})
		MustMatch(t, err.Error(), "D error")
	}

	fn := func(d D) (D, error) {
		return d, nil
	}

	t.Run("run", func(t *testing.T) {
		test(t, fn, c.Run)
	})

	t.Run("run plan", func(t *testing.T) {
		plan, err := c.CreatePlan(fn)
		MustMatch(t, err, nil)

		runner := func(fn any, args ...any) (any, error) {
			return c.RunPlan(plan, args...)
		}
		test(t, fn, runner)
	})

	t.Run("create plan with errors", func(t *testing.T) {
		c := container.New()
		c.Provide(func(bool) string {
			return ""
		})
		_, err := c.CreatePlan(func(bool) {})
		MustNotMatch(t, err, nil)
		MustMatch(t, strings.Contains(err.Error(), "can not create the type: bool"), true,
			"expected missing bool got: %s", err.Error())
	})
}

func TestContainerPlanConcurrent(t *testing.T) {
	type A struct {
		N int
	}
	aFn := func(i int) A {
		return A{N: i}
	}
	type B struct {
		N int
	}
	bFn := func(a A) B {
		return B{N: a.N * 2}
	}
	type C struct {
		N int
	}
	cFn := func(b B) C {
		return C{N: b.N * 3}
	}
	type D struct {
		N int
	}
	dFn := func(c C, i int) (D, error) {
		if c.N < 0 {
			return D{}, errors.New("D error")
		}
		return D{N: c.N*4 + i}, nil
	}

	c := container.New()
	c.Provide(aFn)
	c.Provide(bFn)
	c.Provide(cFn)
	c.Provide(dFn)

	fn := func(d D) (D, error) {
		return d, nil
	}

	plan, err := c.CreatePlan(fn, int(0))
	MustMatch(t, err, nil)

	for i := 0; i < 5; i++ {
		go func(i int) {
			resp, err := c.RunPlan(plan, i)
			MustMatch(t, err, nil)
			d := resp.(D)
			want := (((i * 2) * 3) * 4) + i
			MustMatch(t, d.N, want)
		}(i)
	}

}

func TestContainerPlanIgnore(t *testing.T) {
	type Start struct{}
	type Ctx struct{}

	c := container.New()
	c.Provide(func(Start, http.ResponseWriter) Ctx {
		return Ctx{}
	})

	var respWriterType = reflectUtil.MakeType[http.ResponseWriter]()
	plan, err := c.CreatePlan(func(Ctx) {}, Start{}, respWriterType)
	MustMatch(t, err, nil)

	w := httptest.NewRecorder()
	_, err = c.RunPlan(plan, Start{},
		container.MustCast[http.ResponseWriter](w))
	MustMatch(t, err, nil)
}

func TestContainerPlanOrder(t *testing.T) {
	type Start struct{}
	type Ctx struct{}
	type Apple struct{}
	type Tree struct{}
	type Leaf struct{}
	type Branch struct{}

	c := container.New()
	c.Provide(func(Start) Ctx {
		return Ctx{}
	})
	c.Provide(func(Ctx) Apple {
		return Apple{}
	})

	c.Provide(func(Ctx) Leaf {
		return Leaf{}
	})
	c.Provide(func(Ctx) Branch {
		return Branch{}
	})

	c.Provide(func(Ctx, Apple, Leaf) Tree {
		return Tree{}
	})

	fn := func(t Tree) Tree {
		return t
	}

	plan, err := c.CreatePlan(fn, Start{})
	MustMatch(t, err, nil)

	res, err := c.RunPlan(plan, Start{})
	MustMatch(t, err, nil)
	MustMatch(t, res, Tree{})
}

func BenchmarkContainerRun(b *testing.B) {
	type A struct{}
	a := func() A {
		return A{}
	}
	type B struct{}
	bFn := func(A) B {
		return B{}
	}
	type C struct{}
	cFn := func(B) C {
		return C{}
	}
	type D struct{}
	d := func(C) D {
		return D{}
	}
	type E struct{}
	e := func(D) E {
		return E{}
	}
	type F struct{}
	f := func(E) F {
		return F{}
	}
	endFn := func(F) string {
		return "str"
	}

	c := container.New()
	c.Provide(a)
	c.Provide(bFn)
	c.Provide(cFn)
	c.Provide(d)
	c.Provide(e)
	c.Provide(f)

	b.Run("no container", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			a := a()
			b := bFn(a)
			c := cFn(b)
			d := d(c)
			e := e(d)
			f := f(e)
			endFn(f)
		}
	})

	b.Run("container", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			//nolint
			c.Run(endFn)
		}
	})

	b.Run("container with args", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := c.Run(endFn, "arg", 1, false)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	plan, err := c.CreatePlan(endFn)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("plan", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			//nolint
			c.RunPlan(plan)
		}
	})

	args := make([]any, 100)
	for i := 0; i < len(args); i++ {
		args[i] = i
	}
	b.Run("plan large(100) args", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			//nolint
			c.RunPlan(plan, args...)
		}
	})
}
