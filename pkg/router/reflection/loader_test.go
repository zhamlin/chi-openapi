package reflection

import (
	"fmt"
	"strconv"
	"testing"
)

func failErrT(t tester) func(error) {
	return func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoaderFuncErr(t *testing.T) {
	failErr := failErrT(t)
	c := NewContainer()

	type testStruct struct {
		Value string
	}
	failErr(c.Provide(func(val string) testStruct {
		return testStruct{Value: val}
	}))

	_, err := c.Graph.Sort()
	if err == nil {
		t.Fatal("expected an error")
	}
	t.Log(err)
}

func TestLoaderFunc(t *testing.T) {
	failErr := failErrT(t)
	c := NewContainer()

	failErr(c.Provide(func() string {
		return "10~"
	}))

	failErr(c.Provide(func(v string) (int, error) {
		return strconv.Atoi(v)
	}))

	type testStruct struct {
		Value string
	}
	failErr(c.Provide(func(val int) testStruct {
		return testStruct{Value: fmt.Sprintf("%d", val)}
	}))

	// for k, v := range c.Graph.Verticies {
	// 	t.Logf("vertex: %+v: %+v, %+v\n", k, v.outgoingEdges, v.incomingEdges)
	// }
	// for k, v := range c.Graph.Edges {
	// 	t.Logf("edge: %+v: %+v\n", k, v)
	// }

	_, err := c.Graph.Sort()
	failErr(err)

	// TODO: allow this function to take any amount
	// of values to create "default providers" for this execution context
	t.Log(c.Execute(func(test testStruct) error {
		t.Log(test.Value)
		return fmt.Errorf("blah")
	}))
}

func BenchmarkLoaderFunc(b *testing.B) {
	failErr := failErrT(b)
	c := NewContainer()

	failErr(c.Provide(func() string {
		return "10"
	}))

	failErr(c.Provide(func(v string) (int, error) {
		return strconv.Atoi(v)
	}))

	type testStruct struct {
		Value string
	}
	failErr(c.Provide(func(val int) testStruct {
		return testStruct{Value: fmt.Sprintf("%d", val)}
	}))

	_, err := c.Graph.Sort()
	failErr(err)

	// TODO: allow this function to take any amount
	// of values to create "default providers" for this execution context
	b.Log(c.Execute(func(test testStruct) error {
		b.Log(test.Value)
		return fmt.Errorf("blah")
	}))

	b.Run("test execution speed", func(b *testing.B) {
		b.ReportAllocs()
		for n := 0; n < b.N; n++ {
			c.Execute(func(test testStruct) error {
				return nil
			})
		}
	})
}
