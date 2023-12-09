package router

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/zhamlin/chi-openapi/pkg/openapi3"
)

func trimTrailingSlashes(s string) string {
	if s == "/" {
		return s
	}
	return strings.TrimSuffix(s, "/")
}

func newChiRouteInfo(
	openAPI openapi3.OpenAPI,
	r *http.Request,
) (RouteInfo, bool) {
	ctx := chi.RouteContext(r.Context())
	if ctx == nil {
		return RouteInfo{}, false
	}
	params := make(map[string]string, len(ctx.URLParams.Keys))
	for i, name := range ctx.URLParams.Keys {
		params[name] = ctx.URLParams.Values[i]
	}
	routePattern := trimTrailingSlashes(ctx.RoutePattern())
	info, err := newRouteInfo(routePattern, params, openAPI, r)
	return info, err == nil
}

func newChiRouter() ChiRouter {
	return ChiRouter{
		Router: chi.NewRouter(),
	}
}

type ChiRouter struct {
	chi.Router
}

func (r ChiRouter) Group(fn func(r BaseRouter)) BaseRouter {
	router := r.Router.Group(func(r chi.Router) {
		if fn != nil {
			fn(ChiRouter{r})
		}
	})
	return ChiRouter{router}
}

func (r ChiRouter) Route(pattern string, fn func(r BaseRouter)) BaseRouter {
	router := r.Router.Route(pattern, func(r chi.Router) {
		if fn != nil {
			fn(ChiRouter{r})
		}
	})
	return ChiRouter{router}
}

func (r ChiRouter) With(middlewares ...func(http.Handler) http.Handler) BaseRouter {
	router := r.Router.With(middlewares...)
	return ChiRouter{router}
}
