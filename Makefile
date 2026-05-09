IMAGE ?= minurl
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD)
IMAGE_TAG ?= $(patsubst v%,%,$(VERSION))
COMMIT ?= $(shell git rev-parse --short HEAD)
LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)
DOCKER_VOLUME ?= minurl-data:/data
DOCKER_PORT ?= 8888:8888
NEW_FROM_REV ?= HEAD
OPENAPI_DIR ?= docs/openapi
KIOTA_DIR ?= pkg/kiota/go/gen/client
BIN_DIR ?= bin
OUT_BINARY ?= $(BIN_DIR)/minurl
VERBOSE ?= 0
# Set INTEGRATION_TEST=1 to also run PostgreSQL integration tests (requires Docker).
INTEGRATION_TEST ?= 0

GO_TEST_FLAGS := -race -failfast
ifneq ($(filter 1 yes true y on,$(VERBOSE)),)
GO_TEST_FLAGS += -v
endif

GOLANGCI_LINT_FLAGS := --new-from-rev=$(NEW_FROM_REV)
ifneq ($(filter 1 yes true y on,$(VERBOSE)),)
GOLANGCI_LINT_FLAGS += -v
endif

.PHONY: docker-build
docker-build:
	docker build --build-arg LDFLAGS='$(LDFLAGS)' -t $(IMAGE):$(IMAGE_TAG) .

.PHONY: docker-run
docker-run:
	docker run --rm -p $(DOCKER_PORT) -v $(DOCKER_VOLUME) $(IMAGE):$(IMAGE_TAG)

.PHONY: build
build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(OUT_BINARY) ./cmd/minurl

.PHONY: fix
fix:
	go mod tidy
	golangci-lint run $(GOLANGCI_LINT_FLAGS) --fix ./...

.PHONY: lint-verify
lint-verify:
	golangci-lint config verify

.PHONY: lint
lint: lint-verify
	golangci-lint run $(GOLANGCI_LINT_FLAGS) ./...

.PHONY: test
test:
	INTEGRATION_TEST=$(INTEGRATION_TEST) go test $(GO_TEST_FLAGS) ./...

.PHONY: check-tidy
check-tidy:
	go mod tidy -diff

.PHONY: check
check: check-tidy lint test

.PHONY: ci
ci: check gen
	git diff --exit-code

.PHONY: gen
gen: openapi kiota

.PHONY: openapi
openapi:
	go run ./cmd/minurl openapi --out $(OPENAPI_DIR)

.PHONY: kiota
kiota: openapi
	kiota generate \
		--language Go \
		--openapi $(OPENAPI_DIR)/openapi.json \
		--clean-output \
		--output $(KIOTA_DIR) \
		--class-name MinURLClient \
		--namespace-name github.com/min0625/minurl/$(KIOTA_DIR) \
		--exclude-backward-compatible
