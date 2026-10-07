# Image URL to use all building/pushing image targets
IMG ?= keycloak-operator:dev
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

# Nothing here needs cgo. Disabling it keeps host builds reproducible and avoids
# Apple toolchain linker breakage on macOS.
export CGO_ENABLED = 0

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
	@echo "Syncing CRDs to Helm chart..."
	@/bin/cp -f config/crd/bases/*.yaml charts/keycloak-operator/files/crds/

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
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
OPM ?= $(LOCALBIN)/opm

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.17.2
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v2.11.4
OPERATOR_SDK_VERSION ?= v1.41.1
OPM_VERSION ?= v1.49.0

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

.PHONY: operator-sdk
operator-sdk: $(OPERATOR_SDK) ## Download operator-sdk locally if necessary.
$(OPERATOR_SDK): $(LOCALBIN)
	@[ -f $(OPERATOR_SDK) ] || { \
	set -e ;\
	OS=$$(go env GOOS) ;\
	ARCH=$$(go env GOARCH) ;\
	echo "Downloading operator-sdk $(OPERATOR_SDK_VERSION) for $${OS}/$${ARCH}" ;\
	curl -fsSL -o $(OPERATOR_SDK) "https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH}" ;\
	chmod +x $(OPERATOR_SDK) ;\
	}

.PHONY: opm
opm: $(OPM) ## Download opm locally if necessary.
$(OPM): $(LOCALBIN)
	@[ -f $(OPM) ] || { \
	set -e ;\
	OS=$$(go env GOOS) ;\
	ARCH=$$(go env GOARCH) ;\
	echo "Downloading opm $(OPM_VERSION) for $${OS}/$${ARCH}" ;\
	curl -fsSL -o $(OPM) "https://github.com/operator-framework/operator-registry/releases/download/$(OPM_VERSION)/$${OS}-$${ARCH}-opm" ;\
	chmod +x $(OPM) ;\
	}

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
	@if command -v kind &> /dev/null && kind get clusters 2>/dev/null | grep -q "$(KIND_CLUSTER_NAME)"; then \
		kind load docker-image $(IMG) --name $(KIND_CLUSTER_NAME); \
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
KIND_CONTEXT = kind-$(KIND_CLUSTER_NAME)

##@ Kind Development Cluster
# Simplified workflow: kind-all → kind-redeploy → kind-test-run

.PHONY: kind-check-context
kind-check-context:
	@current_ctx=$$(kubectl config current-context 2>/dev/null || echo "none"); \
	if [ "$$current_ctx" != "$(KIND_CONTEXT)" ]; then \
		echo ""; \
		echo "\033[0;31m[ERROR] Wrong kubectl context!\033[0m"; \
		echo "  Current: $$current_ctx | Expected: $(KIND_CONTEXT)"; \
		echo "  Run: kubectl config use-context $(KIND_CONTEXT)"; \
		echo "  Or create cluster: make kind-all"; \
		echo ""; \
		exit 1; \
	fi

.PHONY: kind-all
kind-all: ## Create Kind cluster and deploy everything (operator + Keycloak).
	./hack/setup-kind.sh all

.PHONY: kind-redeploy
kind-redeploy: kind-check-context ## Rebuild and restart operator (fast iteration).
	@echo "Building and deploying operator..."
	@$(CONTAINER_TOOL) build -t $(IMG) .
	@for node in $$(kind get nodes --name $(KIND_CLUSTER_NAME) 2>/dev/null); do \
		docker exec $$node crictl rmi $(IMG) 2>/dev/null || true; \
	done
	@kind load docker-image $(IMG) --name $(KIND_CLUSTER_NAME)
	@kubectl rollout restart deployment/keycloak-operator -n $(HELM_NAMESPACE)
	@kubectl rollout status deployment/keycloak-operator -n $(HELM_NAMESPACE) --timeout=60s
	@echo "Done"

.PHONY: kind-test-run
kind-test-run: ## Run e2e tests. Use TEST_RUN=TestName to filter.
	USE_EXISTING_CLUSTER=true KEYCLOAK_URL=http://localhost:8080 go test -v -timeout 30m ./test/e2e/... $(if $(TEST_RUN),-run $(TEST_RUN),)

.PHONY: kind-logs
kind-logs: kind-check-context ## Tail operator logs.
	kubectl logs -f -n $(HELM_NAMESPACE) -l app.kubernetes.io/name=keycloak-operator

.PHONY: kind-port-forward
kind-port-forward: kind-check-context ## Port-forward Keycloak to localhost:8080.
	kubectl port-forward svc/keycloak 8080:80 -n keycloak

.PHONY: kind-reset
kind-reset: ## Reset cluster to clean state.
	./hack/setup-kind.sh reset

.PHONY: kind-delete
kind-delete: ## Delete the Kind cluster.
	./hack/setup-kind.sh delete

##@ OLM Bundle (OperatorHub.io and OpenShift community catalog)

# VERSION is the operator version baked into the bundle (no `v` prefix, e.g. 0.8.0).
# Defaults to the Helm chart version so a single source of truth drives releases.
VERSION ?= $(shell yq '.version' charts/keycloak-operator/Chart.yaml)

# Channel selection for the generated bundle. The existing OperatorHub.io entry
# lives on `stable`; staying on the same channel preserves the upgrade graph.
CHANNELS ?= stable
DEFAULT_CHANNEL ?= stable
BUNDLE_CHANNELS := --channels=$(CHANNELS)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image references injected into the CSV.
IMAGE_TAG_BASE ?= ghcr.io/hostzero-gmbh/keycloak-operator
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# Always patch the CSV's containerImage to a digest- or tag-pinned reference
# that the community-operators pipeline can pull. Override IMG on the CLI to
# point at a specific tag (e.g. IMG=ghcr.io/.../keycloak-operator:v0.8.0).
OPERATOR_IMG ?= $(IMAGE_TAG_BASE):v$(VERSION)

# Package name baked into the bundle. Must match the operators/<name>/ directory
# in k8s-operatorhub/community-operators and
# redhat-openshift-ecosystem/community-operators-prod. We pass this on the CLI rather than
# storing it in a PROJECT file because the CSV base in config/manifests/ is
# hand-maintained — there's no scaffolding to keep in sync.
BUNDLE_PACKAGE ?= hostzero-keycloak-operator

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate the OLM bundle under bundle/.
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(OPERATOR_IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS) --package=$(BUNDLE_PACKAGE)
	# Restore the dev image reference in config/manager so local builds aren't affected.
	cd config/manager && $(KUSTOMIZE) edit set image controller=keycloak-operator:dev
	# OperatorHub's pipeline requires metadata.annotations.containerImage to match the
	# image referenced in the deployment. operator-sdk doesn't set it on its own.
	OPERATOR_IMG=$(OPERATOR_IMG) yq -i '.metadata.annotations.containerImage = strenv(OPERATOR_IMG)' bundle/manifests/hostzero-keycloak-operator.clusterserviceversion.yaml
	# relatedImages lets oc-mirror/OLM mirror the image for disconnected clusters. The manager is the only image.
	OPERATOR_IMG=$(OPERATOR_IMG) yq -i '.spec.relatedImages = [{"name": "manager", "image": strenv(OPERATOR_IMG)}]' bundle/manifests/hostzero-keycloak-operator.clusterserviceversion.yaml
	# OpenShift versions the bundle is distributed to (open-ended range). Ignored by OperatorHub.io.
	yq -i '.annotations."com.redhat.openshift.versions" = "v4.12"' bundle/metadata/annotations.yaml
	$(OPERATOR_SDK) bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the OLM bundle image.
	$(CONTAINER_TOOL) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the OLM bundle image.
	$(CONTAINER_TOOL) push $(BUNDLE_IMG)

.PHONY: bundle-run
bundle-run: operator-sdk ## Install the bundle into the current cluster via OLM (for smoke testing).
	$(OPERATOR_SDK) run bundle $(BUNDLE_IMG)
