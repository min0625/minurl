IMAGE ?= minurl
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD)
IMAGE_TAG ?= $(patsubst v%,%,$(VERSION))
COMMIT ?= $(shell git rev-parse --short HEAD)
LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
NEW_FROM_REV ?= HEAD
OPENAPI_DIR ?= docs/openapi

.PHONY: docker-build docker-run fix lint test check-tidy check openapi openapi-json openapi-yaml

docker-build:
	docker build --build-arg LDFLAGS='$(LDFLAGS)' -t $(IMAGE):$(IMAGE_TAG) .

docker-run:
	docker run --rm $(IMAGE):$(IMAGE_TAG)

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
	go run ./cmd/minurl openapi --format=json --out $(OPENAPI_DIR)

openapi-yaml:
	go run ./cmd/minurl openapi --format=yaml --out $(OPENAPI_DIR)
