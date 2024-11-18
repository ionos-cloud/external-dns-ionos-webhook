## Tool Binaries
GO_RUN := go run -modfile ./tools/go.mod

GO_TEST = $(GO_RUN) gotest.tools/gotestsum --format pkgname
GOLANCI_LINT = $(GO_RUN) github.com/golangci/golangci-lint/cmd/golangci-lint
GOFUMPT = $(GO_RUN) mvdan.cc/gofumpt
GORELEASER = $(GO_RUN) github.com/goreleaser/goreleaser/v2


LICENCES_IGNORE_LIST = $(shell cat licences/licences-ignore-list.txt)

ifndef $(GOPATH)
    GOPATH=$(shell go env GOPATH)
    export GOPATH
endif

ARTIFACT_NAME = external-dns-ionos-webhook

# logging
LOG_LEVEL = debug
LOG_ENVIRONMENT = production
LOG_FORMAT = auto


REGISTRY ?= localhost:5001
IMAGE_NAME ?= external-dns-ionos-webhook
IMAGE_TAG ?= latest
IMAGE = $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

show: ## Show variables
	@echo "GOPATH: $(GOPATH)"
	@echo "ARTIFACT_NAME: $(ARTIFACT_NAME)"
	@echo "REGISTRY: $(REGISTRY)"
	@echo "IMAGE_NAME: $(IMAGE_NAME)"
	@echo "IMAGE_TAG: $(IMAGE_TAG)"
	@echo "IMAGE: $(IMAGE)"


##@ Code analysis

.PHONY: fmt
fmt: ## Run go fmt against code.
	$(GOFUMPT) -w .

.PHONY: lint
lint: ## Run golangci-lint against code.
	mkdir -p build/reports
	$(GOLANCI_LINT) run --timeout 5m

.PHONY: lint-with-fix
lint-with-fix: ## Runs linter against all go code with fix.
	mkdir -p build/reports
	$(GOLANCI_LINT) run --fix

.PHONY: static-analysis
static-analysis: lint ## Run static analysis against code.

##@ GO

.PHONY: clean
clean: ## Clean the build directory
	rm -rf ./dist
	rm -rf ./build
	rm -rf ./vendor

.PHONY: build
build: ## Build the binary
	CGO_ENABLED=0 go build -o build/bin/$(ARTIFACT_NAME) ./cmd/webhook

.PHONY: run
run:build ## Run the binary on local machine
	LOG_LEVEL=$(LOG_LEVEL) LOG_ENVIRONMENT=$(LOG_ENVIRONMENT) LOG_FORMAT=$(LOG_FORMAT) build/bin/external-dns-ionos-webhook

##@ Docker

.PHONY: docker-build
docker-build: build ## Build the docker image
	docker build ./ -f localbuild.Dockerfile -t $(IMAGE)

.PHONY: docker-push
docker-push: ## Push the docker image
	docker push $(IMAGE)

##@ Test

.PHONY: unit-test
unit-test: ## Run unit tests
	mkdir -p build/reports
	$(GO_TEST) --junitfile build/reports/unit-test.xml -- -race ./... -count=1 -short -cover -coverprofile build/reports/unit-test-coverage.out


##@ Release

.PHONY: release-check
release-check: ## Check if the release will work
	GITHUB_SERVER_URL=github.com GITHUB_REPOSITORY=ionos-cloud/external-dns-ionos-webhook REGISTRY=$(REGISTRY) IMAGE_NAME=$(IMAGE_NAME) $(GORELEASER) release --snapshot --clean --skip=publish

##@ License

.PHONY: license-check
license-check: ## Run go-licenses check against code.
	go install github.com/google/go-licenses/v2@latest
	mkdir -p build/reports
	echo "$(LICENCES_IGNORE_LIST)"
	go-licenses check --include_tests --ignore "$(LICENCES_IGNORE_LIST)" ./...

.PHONY: license-report
license-report: ## Create licenses report against code.
	go install github.com/google/go-licenses/v2@latest
	mkdir -p build/reports/licenses
	go-licenses report --include_tests --ignore "$(LICENCES_IGNORE_LIST)" ./... >build/reports/licenses/licenses-list.csv
	cat licences/licenses-manual-list.csv >> build/reports/licenses/licenses-list.csv
