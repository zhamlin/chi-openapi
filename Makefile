PACKAGE := github.com/zhamlin/chi-openapi

PHONY: lint
#>> General
lint:
	go vet $(PACKAGE)
	golangci-lint run -E goimports -E maligned -E unparam -E prealloc

PHONY: test
test:
	go test $(PACKAGE)/... -race
