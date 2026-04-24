IMAGE ?= hello-go
TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD)
NEW_FROM_REV ?= HEAD
OPENAPI_DIR ?= docs/openapi

.PHONY: docker-build docker-run fix lint test check-tidy check openapi openapi-json openapi-yaml

docker-build:
	docker build -t $(IMAGE):$(TAG) .

docker-run:
	docker run --rm $(IMAGE):$(TAG)

fix:
	go mod tidy
	golangci-lint run -v --new-from-rev=$(NEW_FROM_REV) --fix ./...

lint:
	golangci-lint run -v --new-from-rev=$(NEW_FROM_REV) ./...

test:
	go test -v -race -failfast ./...

check-tidy:
	go mod tidy -diff

check: check-tidy lint test

openapi: openapi-json openapi-yaml

openapi-json:
	go run . openapi --format=json --out $(OPENAPI_DIR)

openapi-yaml:
	go run . openapi --format=yaml --out $(OPENAPI_DIR)
