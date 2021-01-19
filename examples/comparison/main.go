package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/zhamlin/chi-openapi/pkg/openapi/operations"
	"github.com/zhamlin/chi-openapi/pkg/router"
	"github.com/zhamlin/chi-openapi/pkg/router/reflection"

	"github.com/go-chi/chi"
)

type basicPostRequestBody struct {
}

type basicPostParams struct {
}

type basicPostRequest struct {
	Body   basicPostRequestBody
	Params basicGetParams
}
type basicPostResponse struct{}

type basicGetResponse struct {
	NameParam string `json:"name"`
}

type basicGetParams struct {
	Name string `query:"name" doc:"helpful information here"`
}

func basicGET(w http.ResponseWriter, r *http.Request) {
	nameFromQuery := r.URL.Query().Get("name")
	resp := basicGetResponse{NameParam: nameFromQuery}

	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		panic(err)
	}
}

func basicPOST(w http.ResponseWriter, r *http.Request) {
}

func chiRouter() http.Handler {
	r := chi.NewRouter()
	r.Get("/", http.HandlerFunc(basicGET))
	r.Post("/", http.HandlerFunc(basicPOST))
	return r
}

func chiOpenAPIRouter() http.Handler {
	r := router.NewRouter()
	r.Get("/", basicGET, []operations.Option{
		operations.Params(basicGetParams{}),
		operations.JSONResponse(http.StatusOK, "ok", basicGetResponse{}),
	})
	r.Post("/", basicPOST, []operations.Option{
		operations.JSONResponse(http.StatusCreated, "created a resource", basicGetResponse{}),
	})
	str, err := r.GenerateSpec()
	if err != nil {
		panic(err)
	}
	fmt.Printf("github.com/zhamlin/chi-openapi spec:\n%s\n", str)
	return r
}

func chiReflectOpenAPIRouter() http.Handler {
	r := reflection.NewRouter()
	// TODO: swap basic http handler to custom func
	r.Get("/", basicGET, []operations.Option{
		operations.Params(basicGetParams{}),
		operations.JSONResponse(http.StatusOK, "ok", basicGetResponse{}),
	})
	r.Post("/", basicPOST, []operations.Option{
		operations.JSONResponse(http.StatusCreated, "created a resource", basicGetResponse{}),
	})
	return r
}

func testPostRequest(normal, openapi, reflection http.Handler) {
	// TODO
}

func testGetRequest(normal, openapi, reflection http.Handler) {
	queryNameValue := "test_name"
	getRequest := httptest.NewRequest(http.MethodGet, "/?name="+queryNameValue, nil)

	// normal chi router
	{
		recorder := httptest.NewRecorder()
		getResponse := basicGetResponse{}
		normal.ServeHTTP(recorder, getRequest)
		if err := json.NewDecoder(recorder.Body).Decode(&getResponse); err != nil {
			panic(err)
		}
		if getResponse.NameParam != queryNameValue {
			panic("wrong name")
		}
	}
	// github.com/zhamlin/chi-openapi router
	{
		recorder := httptest.NewRecorder()
		getResponse := basicGetResponse{}
		openapi.ServeHTTP(recorder, getRequest)
		if err := json.NewDecoder(recorder.Body).Decode(&getResponse); err != nil {
			panic(err)
		}
		if getResponse.NameParam != queryNameValue {
			panic("wrong name")
		}
	}

	// github.com/zhamlin/chi-openapi reflection router
	{
		recorder := httptest.NewRecorder()
		getResponse := basicGetResponse{}
		reflection.ServeHTTP(recorder, getRequest)
		if err := json.NewDecoder(recorder.Body).Decode(&getResponse); err != nil {
			panic(err)
		}
		if getResponse.NameParam != queryNameValue {
			panic("wrong name")
		}
	}
}

func main() {
	normalChiRouter := chiRouter()
	chiOpenAPIRouter := chiOpenAPIRouter()
	reflectRouter := chiReflectOpenAPIRouter()

	testGetRequest(normalChiRouter, chiOpenAPIRouter, reflectRouter)
	testPostRequest(normalChiRouter, chiOpenAPIRouter, reflectRouter)
}
