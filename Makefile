.PHONY: build
build: tidy ## Build the CLI
	go build

build-image: ## Build the Docker image
	docker build -t kubernetes-event-exporter .

.PHONY: fmt
fmt: ## Run go fmt against code
	gofmt -s -l -w .

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

tidy: ## Run go mod tidy
	go mod tidy

test: tidy ## Run unit tests
	go test -cover -mod=mod -v ./...

# Kubernetes versions for the kind-based e2e matrix. Override on the CLI, e.g.
#   make e2e-kind KIND_NODE_IMAGE=kindest/node:v1.31.0
KIND_CLUSTER ?= kee-e2e
KIND_NODE_IMAGE ?= kindest/node:v1.30.0

.PHONY: e2e
e2e: ## Run e2e tests against the current kube context (cluster must exist)
	go test -tags e2e -v -timeout 15m ./test/e2e/...

.PHONY: smoke
smoke: ## Run the deploy smoke test against the current kind cluster
	KIND_CLUSTER=$(KIND_CLUSTER) ./test/e2e/smoke.sh

.PHONY: e2e-kind
e2e-kind: ## Create a throwaway kind cluster, run e2e + smoke, then tear it down
	kind create cluster --name $(KIND_CLUSTER) --image $(KIND_NODE_IMAGE) --wait 120s
	$(MAKE) e2e || { kind delete cluster --name $(KIND_CLUSTER); exit 1; }
	$(MAKE) smoke || { kind delete cluster --name $(KIND_CLUSTER); exit 1; }
	kind delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

clean: ## Delete go.sum and clean mod cache
	go clean -modcache
	rm go.sum

.PHONY: help
help: ## Display this help.
	@cat $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } '
