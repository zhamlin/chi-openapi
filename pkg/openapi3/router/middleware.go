package router

import (
	"context"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/zhamlin/chi-openapi/pkg/openapi3"
)

type RouteInfo struct {
	Request     *http.Request
	QueryValues url.Values

	OpenAPI openapi3.OpenAPI

	PathItem  openapi3.PathItem
	Operation openapi3.Operation
	URLParams map[string]string
}

func newRouteInfo(openAPI openapi3.OpenAPI, r *http.Request) (RouteInfo, bool) {
	ctx := chi.RouteContext(r.Context())
	if ctx == nil {
		return RouteInfo{}, false
	}

	routePattern := ctx.RoutePattern()
	if routePattern == "" {
		// RoutePattern removes any trailing slashes, so an empty
		// string _could_ be a /
		routePattern = "/"
	}
	item, has := openAPI.GetPath(routePattern)
	if !has {
		return RouteInfo{}, false
	}

	op, has := item.GetOperation(r.Method)
	if !has {
		return RouteInfo{}, false
	}

	info := RouteInfo{
		Request:     r,
		QueryValues: r.URL.Query(),
		OpenAPI:     openAPI,

		Operation: op,
		PathItem:  item,
		URLParams: map[string]string{},
	}
	for i, name := range ctx.URLParams.Keys {
		info.URLParams[name] = ctx.URLParams.Values[i]
	}
	return info, true
}

type routeInfoCtxKey struct{}

func GetRouteInfo(ctx context.Context) (RouteInfo, bool) {
	info, has := ctx.Value(routeInfoCtxKey{}).(RouteInfo)
	return info, has
}

func SetRouteInfo(ctx context.Context, info RouteInfo) context.Context {
	return context.WithValue(ctx, routeInfoCtxKey{}, info)
}

func addRouteInfo(router *Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if info, has := newRouteInfo(router.spec, r); has {
				r = r.WithContext(SetRouteInfo(r.Context(), info))
			}
			next.ServeHTTP(w, r)
		})
	}
}
