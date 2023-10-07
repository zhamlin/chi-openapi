package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	. "github.com/zhamlin/chi-openapi/internal/testing"
	. "github.com/zhamlin/chi-openapi/pkg/openapi3/operations"
)

func TestGetRouteInfo(t *testing.T) {
	r := NewRouter(Config{})
	r.Get("/", nil,
		Summary("the operation"),
		Params(struct{}{}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	// set the chi context with the correct info
	ctx := req.Context()
	chiCtx := chi.NewRouteContext()
	chiCtx.RoutePatterns = []string{"/"}
	chiCtx.URLParams.Keys = []string{"id"}
	chiCtx.URLParams.Values = []string{"1"}
	ctx = context.WithValue(ctx, chi.RouteCtxKey, chiCtx)

	info, has := newRouteInfo(r.OpenAPI(), req.WithContext(ctx))
	MustMatch(t, has, true, "expected an operation to be found")
	MustMatch(t, info.Operation.Summary, "the operation")
	MustMatch(t, info.Operation.Operation == nil, false)
	MustMatch(t, info.URLParams, map[string]string{"id": "1"})

	req = httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	_, has = newRouteInfo(r.OpenAPI(), req)
	MustMatch(t, has, false, "POST not in openapi spec for /")
}

func TestTrimTrailingSlashes(t *testing.T) {
	tests := []struct {
		have string
		want string
	}{
		{
			have: "/",
			want: "/",
		},
		{
			have: "/foo",
			want: "/foo",
		},
		{
			have: "/foo/",
			want: "/foo",
		},
	}

	for _, test := range tests {
		result := trimTrailingSlashes(test.have)
		MustMatch(t, result, test.want)
	}
}
