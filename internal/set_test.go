package internal_test

import (
	"testing"

	"github.com/zhamlin/chi-openapi/internal"

	. "github.com/zhamlin/chi-openapi/internal/testing"
)

func TestSet(t *testing.T) {
	s := internal.NewSet[int]()
	s.Add(1)
	s.Add(2)
	s.Add(2)
	s.Add(3)

	mustHave := []int{1, 2, 3}
	for _, i := range mustHave {
		MustMatch(t, s.Has(i), true)
	}
	MustMatch(t, len(s.Items()), len(mustHave))
}
