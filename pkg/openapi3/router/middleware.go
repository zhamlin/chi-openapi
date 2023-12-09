package router

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/zhamlin/chi-openapi/pkg/openapi3"
)

type RouteInfo struct {
	Request     *http.Request
	QueryValues url.Values
	OpenAPI     openapi3.OpenAPI
	PathItem    openapi3.PathItem
	Operation   openapi3.Operation
	URLParams   map[string]string
}

func newRouteInfo(
	routePattern string,
	params map[string]string,
	openAPI openapi3.OpenAPI,
	r *http.Request,
) (RouteInfo, error) {
	item, has := openAPI.GetPath(routePattern)
	if !has {
		return RouteInfo{}, fmt.Errorf("path not found: %s", routePattern)
	}

	op, has := item.GetOperation(r.Method)
	if !has {
		return RouteInfo{}, fmt.Errorf("operation not found for method: %s", r.Method)
	}

	return RouteInfo{
		Request:     r,
		QueryValues: r.URL.Query(),
		OpenAPI:     openAPI,
		Operation:   op,
		PathItem:    item,
		URLParams:   params,
	}, nil
}

type routeInfoCtxKey struct{}

func GetRouteInfo(ctx context.Context) (RouteInfo, bool) {
	info, has := ctx.Value(routeInfoCtxKey{}).(RouteInfo)
	return info, has
}

func SetRouteInfo(ctx context.Context, info RouteInfo) context.Context {
	return context.WithValue(ctx, routeInfoCtxKey{}, info)
}

type routeInfoFn func(openapi3.OpenAPI, *http.Request) (RouteInfo, bool)

func addRouteInfo(router *Router, newRouteInfo routeInfoFn) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if info, has := newRouteInfo(router.spec, r); has {
				r = r.WithContext(SetRouteInfo(r.Context(), info))
			}
			next.ServeHTTP(w, r)
		})
	}
}
