PHONY: lint
#>> General
lint:
	go vet chi-openapi/...
	golangci-lint run -E goimports -E maligned -E unparam -E prealloc
