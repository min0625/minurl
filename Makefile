IMAGE ?= hello-go
TAG ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD)
COMMIT ?= $(shell git rev-parse --short HEAD)
LDFLAGS ?= -s -w -X github.com/min0625/minurl/cmd/minurl.version=$(TAG) -X github.com/min0625/minurl/cmd/minurl.commit=$(COMMIT)
NEW_FROM_REV ?= HEAD
OPENAPI_DIR ?= docs/openapi

.PHONY: docker-build docker-run fix lint test check-tidy check openapi openapi-json openapi-yaml

docker-build:
	docker build --build-arg LDFLAGS='$(LDFLAGS)' -t $(IMAGE):$(TAG) .

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
	go run ./cmd/minurl openapi --format=json --out $(OPENAPI_DIR)

openapi-yaml:
	go run ./cmd/minurl openapi --format=yaml --out $(OPENAPI_DIR)
