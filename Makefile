# Image URL to use all building/pushing image targets
IMG ?= keycloak-operator:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests (requires Kind cluster with operator and port-forward).
	go test -v -timeout 30m ./test/e2e/...

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Documentation

.PHONY: docs
docs: ## Build the documentation (requires mdBook).
	cd docs && mdbook build

.PHONY: docs-serve
docs-serve: ## Serve documentation locally with hot reload.
	cd docs && mdbook serve --open

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for cross-platform support
	- $(CONTAINER_TOOL) buildx create --use
	$(CONTAINER_TOOL) buildx build --push --platform linux/amd64,linux/arm64 -t ${IMG} .

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.17.2
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v2.8.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and target.
define go-install-tool
@[ -f $(1) ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
}
endef

##@ Helm

HELM_CHART_DIR = charts/keycloak-operator
HELM_RELEASE_NAME ?= keycloak-operator
HELM_NAMESPACE ?= keycloak-operator

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	helm lint $(HELM_CHART_DIR)

.PHONY: helm-template
helm-template: ## Render chart templates locally for debugging.
	helm template $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) --namespace $(HELM_NAMESPACE)

.PHONY: helm-install
helm-install: ## Install the Helm chart.
	helm upgrade --install $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) \
		--namespace $(HELM_NAMESPACE) \
		--create-namespace

.PHONY: helm-install-dev
helm-install-dev: docker-build ## Install the Helm chart with dev values (builds and loads local image).
	@if command -v kind &> /dev/null && kind get clusters 2>/dev/null | grep -q .; then \
		kind load docker-image $(IMG); \
	elif command -v minikube &> /dev/null && minikube status &> /dev/null; then \
		minikube image load $(IMG); \
	fi
	helm upgrade --install $(HELM_RELEASE_NAME) $(HELM_CHART_DIR) \
		--namespace $(HELM_NAMESPACE) \
		--create-namespace \
		-f $(HELM_CHART_DIR)/values-dev.yaml \
		--set image.repository=$(word 1,$(subst :, ,$(IMG))) \
		--set image.tag=$(word 2,$(subst :, ,$(IMG)))

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall the Helm chart.
	helm uninstall $(HELM_RELEASE_NAME) --namespace $(HELM_NAMESPACE)

.PHONY: helm-package
helm-package: ## Package the Helm chart.
	helm package $(HELM_CHART_DIR)

.PHONY: helm-docs
helm-docs: ## Generate Helm documentation (requires helm-docs).
	@command -v helm-docs >/dev/null 2>&1 || { echo "helm-docs not installed. Install with: go install github.com/norwoodj/helm-docs/cmd/helm-docs@latest"; exit 1; }
	helm-docs --chart-search-root=$(HELM_CHART_DIR)

##@ Kind Cluster

KIND_CLUSTER_NAME ?= keycloak-operator-dev

.PHONY: kind-create
kind-create: ## Create a Kind cluster for local development.
	./hack/setup-kind.sh create

.PHONY: kind-delete
kind-delete: ## Delete the Kind cluster.
	./hack/setup-kind.sh delete

.PHONY: kind-reset
kind-reset: ## Reset (delete and recreate) the Kind cluster.
	./hack/setup-kind.sh reset

.PHONY: kind-status
kind-status: ## Show Kind cluster status.
	./hack/setup-kind.sh status

.PHONY: kind-load
kind-load: docker-build ## Build and load the operator image into Kind.
	kind load docker-image $(IMG) --name $(KIND_CLUSTER_NAME)

.PHONY: kind-deploy
kind-deploy: kind-load install helm-install-dev ## Deploy operator to Kind cluster.
	@echo "Operator deployed to Kind cluster"

.PHONY: kind-deploy-keycloak
kind-deploy-keycloak: ## Deploy Keycloak to Kind cluster.
	./hack/setup-kind.sh deploy-keycloak

.PHONY: kind-all
kind-all: ## Create Kind cluster and deploy everything (operator + Keycloak).
	./hack/setup-kind.sh all

.PHONY: kind-logs
kind-logs: ## Tail operator logs in Kind cluster.
	kubectl logs -f -n $(HELM_NAMESPACE) -l app.kubernetes.io/name=keycloak-operator

.PHONY: kind-test
kind-test: ## Run all tests (unit + e2e) against Kind cluster.
	./hack/setup-kind.sh test-e2e

.PHONY: kind-port-forward
kind-port-forward: ## Port-forward Keycloak from Kind cluster to localhost:8080.
	kubectl port-forward svc/keycloak 8080:80 -n keycloak
