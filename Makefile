IMAGE ?= minurl
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || git rev-parse --short HEAD)
# Leading "v" is stripped at every use, so a VERSION given on the command line is
# normalized too: release.yml publishes 1.2.3 for tag v1.2.3, and both the image
# tag and `minurl version` have to match that.
IMAGE_TAG ?= $(patsubst v%,%,$(VERSION))
COMMIT ?= $(shell git rev-parse HEAD)
LDFLAGS ?= -s -w -X main.version=$(patsubst v%,%,$(VERSION)) -X main.commit=$(COMMIT)
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

# An unresolvable rev is not silent: golangci-lint warns, reports every issue, and
# exits 1. This guard is for the message, not the exit code.
.PHONY: check-rev
check-rev:
	@git rev-parse --verify --quiet '$(NEW_FROM_REV)^{commit}' >/dev/null || \
		{ echo "NEW_FROM_REV=$(NEW_FROM_REV) is not a valid revision" >&2; exit 1; }

# Tidy first: a stale go.mod fails typecheck, which is reported past --new-from-rev
# while every other linter falls silent -- so --fix would quietly repair nothing.
# Tidy again after: exptostd rewrites x/exp imports to stdlib and strips a require.
# The closing lint reports whatever --fix could not repair.
.PHONY: fix
fix: check-rev
	go mod tidy
	golangci-lint run $(GOLANGCI_LINT_FLAGS) --fix ./...
	go mod tidy
	@$(MAKE) --no-print-directory lint

.PHONY: lint
lint: check-rev
	golangci-lint config verify
	golangci-lint run $(GOLANGCI_LINT_FLAGS) ./...

.PHONY: test
test:
	INTEGRATION_TEST=$(INTEGRATION_TEST) go test $(GO_TEST_FLAGS) ./...

.PHONY: check-tidy
check-tidy:
	go mod tidy -diff

# The full gate: every hook in .pre-commit-config.yaml -- check-tidy, lint and test
# included -- over every tracked file, i.e. what `git commit` runs but repo-wide.
# NEW_FROM_REV, INTEGRATION_TEST and VERBOSE given on the command line reach the
# nested makes through MAKEFLAGS, which prek's subprocesses inherit; without
# NEW_FROM_REV lint falls back to HEAD and reports nothing on CI's clean checkout.
.PHONY: check
check:
	prek run --all-files --show-diff-on-failure

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
