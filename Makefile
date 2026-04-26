IMAGE ?= minurl
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD)
IMAGE_TAG ?= $(patsubst v%,%,$(VERSION))
COMMIT ?= $(shell git rev-parse --short HEAD)
LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
DOCKER_VOLUME ?= minurl-data:/data
DOCKER_PORT ?= 8888:8888
NEW_FROM_REV ?= HEAD
OPENAPI_DIR ?= docs/openapi
BIN_DIR ?= bin
OUT_BINARY ?= $(BIN_DIR)/minurl

.PHONY: docker-build docker-run fix lint test check-tidy check-openapi check openapi build

docker-build:
	docker build --build-arg LDFLAGS='$(LDFLAGS)' -t $(IMAGE):$(IMAGE_TAG) .

docker-run:
	docker run --rm -p $(DOCKER_PORT) -v $(DOCKER_VOLUME) $(IMAGE):$(IMAGE_TAG)

fix:
	go mod tidy
	golangci-lint run -v --new-from-rev=$(NEW_FROM_REV) --fix ./...

lint:
	golangci-lint run -v --new-from-rev=$(NEW_FROM_REV) ./...

test:
	go test -v -race -failfast ./...

check-tidy:
	go mod tidy -diff

check-openapi:
	@tmp_dir=$$(mktemp -d); \
	trap 'rm -rf "$$tmp_dir"' EXIT; \
	$(MAKE) openapi OPENAPI_DIR=$$tmp_dir >/dev/null; \
	cmp -s $(OPENAPI_DIR)/openapi.json $$tmp_dir/openapi.json && \
	cmp -s $(OPENAPI_DIR)/openapi.yaml $$tmp_dir/openapi.yaml && \
	echo "OpenAPI docs are up to date." || \
	(echo "OpenAPI docs are out of date. Run 'make openapi' and commit updated files." && exit 1)

check: check-tidy lint test check-openapi

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(OUT_BINARY) -ldflags '$(LDFLAGS)' ./cmd/minurl

openapi:
	go run ./cmd/minurl openapi --out $(OPENAPI_DIR)
