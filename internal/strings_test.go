package internal_test

import (
	"testing"

	"github.com/zhamlin/chi-openapi/internal"
	. "github.com/zhamlin/chi-openapi/internal/testing"
)

func TestTrimString(t *testing.T) {
	tests := []struct {
		name string
		have string
		want string
	}{
		{
			have: `test`,
			want: "test",
		},
		{
			have: `
                test
            `,
			want: "test",
		},
		{
			have: `
        multiple lines  
                still work fine  
            `,
			want: "multiple lines\nstill work fine",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := internal.TrimString(test.have)
			if result != test.want {
				t.Fatalf("got:\t%s\nwanted:\t%s", result, test.want)
			}
		})
	}
}

func TestBoolFromString(t *testing.T) {
	tests := []struct {
		name    string
		have    string
		want    bool
		wantErr bool
	}{
		{
			have: "true",
			want: true,
		},
		{
			have: "false",
			want: false,
		},
		{
			have:    "false1",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := internal.BoolFromString(test.have)
			if test.wantErr {
				MustNotMatch(t, err, nil)
			} else {
				MustMatch(t, err, nil)
			}
			MustMatch(t, test.want, result)
		})
	}
}
