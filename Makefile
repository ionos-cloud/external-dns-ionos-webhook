## Tool Binaries
GO_RUN := go run -modfile ./tools/go.mod

BUILD_DIR := ./build

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

KIND_CLUSTER_NAME = external-dns
KIND_CLUSTER_CONFIG = ./deployments/kind/cluster.yaml
KIND_CLUSTER_RUNNING ?= $(shell kind get clusters | grep $(KIND_CLUSTER_NAME))
KIND_CLUSTER_WAIT = 60s

MOCKSERVER_RUNNING ?= $(shell docker ps -q --filter "name=mockserver")
EXTERNAL_DNS_RUNNING ?= $(shell docker ps -q --filter "name=externaldns")

EXTERNALDNS_IMAGE = registry.k8s.io/external-dns/external-dns:v0.15.0

PSEUDO_IONOS_CLOUD_API_KEY = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxMjMsImVtYWlsIjoidXNlckBleGFtcGxlLmNvbSIsImV4cCI6MTYwOTcyMzQ2MCwiaWF0IjoxNjA5NzIyODYwfQ.nKZ8eIGFEnkCZ4yarPPde23hYzLHhqn9Od_L-X0jf0g"

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
	@echo "LOG_LEVEL: $(LOG_LEVEL)"
	@echo "KIND_CLUSTER_NAME: $(KIND_CLUSTER_NAME)"
	@echo "KIND_CLUSTER_RUNNING: $(KIND_CLUSTER_RUNNING)"
	@echo "KIND_CLUSTER_CONFIG: $(KIND_CLUSTER_CONFIG)"
	@echo "MOCKSERVER_RUNNING: $(MOCKSERVER_RUNNING)"
	@echo "EXTERNAL_DNS_RUNNING: $(EXTERNAL_DNS_RUNNING)"


##@ Code analysis

.PHONY: fmt
fmt: ## Run go fmt against code.
	$(GOFUMPT) -w .

.PHONY: lint
lint: ## Run golangci-lint against code.
	mkdir -p $(BUILD_DIR)/reports
	$(GOLANCI_LINT) run --timeout 5m

.PHONY: lint-with-fix
lint-with-fix: ## Runs linter against all go code with fix.
	mkdir -p $(BUILD_DIR)/reports
	$(GOLANCI_LINT) run --fix

.PHONY: static-analysis
static-analysis: lint ## Run static analysis against code.

##@ GO

.PHONY: clean
clean: ## Clean the build directory
	rm -rf ./dist
	rm -rf $(BUILD_DIR)
	rm -rf ./vendor

.PHONY: build
build: ## Build the binary
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/bin/$(ARTIFACT_NAME) ./cmd/webhook

.PHONY: run
run:build ## Run the binary on local machine
	LOG_LEVEL=$(LOG_LEVEL) LOG_ENVIRONMENT=$(LOG_ENVIRONMENT) LOG_FORMAT=$(LOG_FORMAT) $(BUILD_DIR)/bin/external-dns-ionos-webhook

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
	mkdir -p $(BUILD_DIR)/reports
	$(GO_TEST) --junitfile $(BUILD_DIR)/reports/unit-test.xml -- -tags=unit -race ./... -count=1 -short -cover -coverprofile $(BUILD_DIR)/reports/unit-test-coverage.out

.PHONY: integration-test
integration-test: ## Run integration tests
	mkdir -p $(BUILD_DIR)/reports
	$(GO_TEST) --junitfile $(BUILD_DIR)/reports/integration-test.xml -- -tags=integration ./... -count=1 -cover -coverprofile $(BUILD_DIR)/reports/integration-test-coverage.out

##@ Release

.PHONY: release-check
release-check: ## Check if the release will work
	GITHUB_SERVER_URL=github.com GITHUB_REPOSITORY=ionos-cloud/external-dns-ionos-webhook REGISTRY=$(REGISTRY) IMAGE_NAME=$(IMAGE_NAME) $(GORELEASER) release --snapshot --clean --skip=publish

##@ License

.PHONY: license-check
license-check: ## Run go-licenses check against code.
	go install github.com/google/go-licenses@v1.6.0
	mkdir -p $(BUILD_DIR)/reports
	echo "$(LICENCES_IGNORE_LIST)"
	go-licenses check --include_tests --ignore "$(LICENCES_IGNORE_LIST)" ./...

.PHONY: license-report
license-report: ## Create licenses report against code.
	go install github.com/google/go-licenses@v1.6.0
	mkdir -p $(BUILD_DIR)/reports/licenses
	go-licenses report --include_tests --ignore "$(LICENCES_IGNORE_LIST)" ./... >$(BUILD_DIR)/reports/licenses/licenses-list.csv
	cat licences/licenses-manual-list.csv >> $(BUILD_DIR)/reports/licenses/licenses-list.csv

.PHONY: kind
kind: ## Create a kind cluster if not exists
# if KIND_CLUSTER_RUNNING is empty, then create the cluster
ifeq ($(KIND_CLUSTER_RUNNING),)
	kind create cluster --name $(KIND_CLUSTER_NAME) --config $(KIND_CLUSTER_CONFIG)
	mkdir -p $(BUILD_DIR)/kind
	kind get kubeconfig --name $(KIND_CLUSTER_NAME) > $(BUILD_DIR)/kind/config
endif

.PHONY: kind-delete
kind-delete: ## Delete the kind cluster
ifeq ($(KIND_CLUSTER_RUNNING),$(KIND_CLUSTER_NAME))
	kind delete cluster --name $(KIND_CLUSTER_NAME)
	rm -rf $(BUILD_DIR)/kind
endif

.PHONY: external-dns
external-dns: ## run external-dns
# only if kind is running
ifeq ($(KIND_CLUSTER_RUNNING),$(KIND_CLUSTER_NAME))
ifeq ($(EXTERNAL_DNS_RUNNING),)
	docker run -d --restart=on-failure --network host --name externaldns -v $(BUILD_DIR)/kind/config:/root/.kube/config \
	$(EXTERNALDNS_IMAGE) --source=ingress --provider=webhook --log-level=$(LOG_LEVEL)
endif
else
	@echo "Kind cluster is not running"
	@exit 1
endif

.PHONY: external-dns-delete
external-dns-delete: ## Stop and delete external-dns
	docker rm -f externaldns


.PHONY: mockserver
mockserver: ## Run mockserver
ifeq ($(MOCKSERVER_RUNNING),)
	docker run -d --network host --name mockserver -p 1080:1080 -e MOCKSERVER_LOG_LEVEL=DEBUG  mockserver/mockserver:5.15.0
endif
	./scripts/mockserver/mockserver_stubs.sh

.PHONY: mockserver-dashboard
mockserver-dashboard: mockserver ## Open mockserver dashboard
	open http://localhost:1080/mockserver/dashboard

.PHONY: mockserver-delete
mockserver-delete: ## Stop and delete mockserver
	docker rm -f mockserver

.PHONY: run-ionos-cloud-webhook
run-ionos-cloud-webhook: mockserver external-dns ## Run the webhook with ionos-cloud provider with mockserver
	LOG_LEVEL=debug \
	LOG_FORMAT=text \
	SERVER_HOST=localhost \
	SERVER_PORT=8888 \
	SERVER_READ_TIMEOUT= \
	SERVER_WRITE_TIMEOUT= \
	DOMAIN_FILTER= \
	EXCLUDE_DOMAIN_FILTER= \
	REGEXP_DOMAIN_FILTER= \
	REGEXP_DOMAIN_FILTER_EXCLUSION= \
	IONOS_API_KEY=$(PSEUDO_IONOS_CLOUD_API_KEY) \
	IONOS_API_URL="http://localhost:1080" \
	IONOS_AUTH_HEADER= \
	IONOS_DEBUG=true \
	build/bin/external-dns-ionos-webhook
