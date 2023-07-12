package internal_test

import (
	"testing"

	"github.com/zhamlin/chi-openapi/internal"
	. "github.com/zhamlin/chi-openapi/internal/testing"
)

func TestUnique(t *testing.T) {
	tests := []struct {
		name string
		have []int
		want []int
	}{
		{
			have: []int{1, 2, 3, 3, 2, 1},
			want: []int{1, 2, 3},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := internal.Unique(test.have)
			MustMatch(t, res, test.want)
		})
	}
}

func TestReverse(t *testing.T) {
	tests := []struct {
		name string
		have []int
		want []int
	}{
		{
			have: []int{1, 2, 3},
			want: []int{3, 2, 1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			internal.Reverse(test.have)
			MustMatch(t, test.have, test.want)
		})
	}
}
