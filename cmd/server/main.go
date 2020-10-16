package main

import (
	"fmt"
	"net/http"

	"chi-openapi/pkg/openapi"
	"chi-openapi/pkg/router"
)

// TODO: enum support
type StatusType int

const (
	StatusNew StatusType = 0
	StatusOld StatusType = 1
	StatusQA  StatusType = 2
)

type Request struct {
	Name       string       `json:"name"`
	Age        int64        `json:"age"`
	Float      float64      `json:"float"`
	Status     StatusType   `json:"status"`
	Collection []Collection `json:"collection"`
	String     []string     `json:"strings"`
	Ints       []int        `json:"ints"`
	Apples     []Apple      `json:"apples"`
	Bananas    Bananas      `json:"bananas"`
	Bananases  Bananas      `json:"bananases"`
}

func (Request) SchemaID() string {
	return "request"
}

type Collection interface {
	isCollection()
}

type Apple struct {
	Color int `json:"color"`
}

func (Apple) isCollection() {}

func (Apple) SchemaID() string {
	return "Noway"
}

type Bananas []Banana
type Banana struct {
	Color string `json:"color"`
}

func (Banana) isCollection() {}

func (Banana) schemaID() string {
	return "banananane"
}

func NewGetAirPlaneParams(r *http.Request) GetAirplaneParams {
	return GetAirplaneParams{}
}

type GetAirplaneParams struct {
	//@doc some description here that will be copied over
	//@doc more sutff here
	// well this is awkard, won't be included!
	ID   string   `path:"id" doc:"uuid that does the thing" required:"true" format:"uuid"`
	All  bool     `query:"all"`
	Tags []string `query:"tags"`
}

// TODO: code gen, take @doc comments and inject
func testHandler(w http.ResponseWriter, r *http.Request) {
	NewGetAirPlaneParams(r)
}

func paramWrapper(w http.ResponseWriter, r *http.Request, p GetAirplaneParams) error {
	return nil
}

func main() {
	// reflector := jsonschema.Reflector{
	// 	RequiredFromJSONSchemaTags: true,
	// }
	// def := reflector.Reflect(GetAirplaneParams{})
	// // def.Definitions[0].Properties[]
	// b, _ := def.MarshalJSON()
	// var prettyJSON bytes.Buffer
	// json.Indent(&prettyJSON, b, "", " ")

	// fmt.Println(prettyJSON.String())
	r := router.NewRouter(openapi.Info{
		Title:       "Some airplane service",
		Description: "Give you all of the airplane details",
		Version:     "v1.1.0",
	})
	// r.Get("/airplanes/{id}", router.OperationOptions{
	// 	router.Summary("Get an airplane by id"),
	// 	router.Params(GetAirplaneParams{}),
	// 	router.JSONResponse(http.StatusOK, "success", Request{}),
	// 	router.JSONResponse(http.StatusBadRequest, "bad request", nil),
	// }, testHandler)
	// r.Post("/airplanes", router.OperationOptions{
	// 	router.Summary("Create an airplane"),
	// 	// router.Body("airplane body", Request{}),
	// 	router.JSONResponse(http.StatusCreated, "success", Request{}),
	// 	// router.JSONResponse(http.StatusBadRequest, "bad request", nil),
	// }, testHandler)

	r.Route("/v1", func(r *router.Router) {
		r.Route("/another-level", func(r *router.Router) {
			r.Post("/airplanes", router.OperationOptions{
				router.Summary("Create an airplane"),
				// router.Body("airplane body", Request{}),
				router.JSONResponse(http.StatusCreated, "success", Request{}),
				// router.JSONResponse(http.StatusBadRequest, "bad request", nil),
			}, testHandler)
		})
	})

	fmt.Println(r.GenerateSpec())
}
