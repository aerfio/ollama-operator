# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.30.0
KO_DOCKER_REPO ?= ko.local
CURRENT_DIR = $(dir $(abspath $(firstword $(MAKEFILE_LIST))))

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

ifeq ($(OS),Windows_NT)
	detected_OS := Windows
else
	detected_OS := $(shell sh -c 'uname 2>/dev/null || echo Unknown')
endif

.PHONY: all
all: generate build test lint-fix lint-chainsaw-tests

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: generate
generate: manifests generate-deep-copy k8s-client-gen k8s-gvk-gen

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd paths="$(CURRENT_DIR)/..." output:crd:artifacts:config="$(CURRENT_DIR)/helm/chart/ollama-operator/templates/crds"

.PHONY: generate-deep-copy
generate-deep-copy: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object paths="$(CURRENT_DIR)/..."

.PHONY: test
test: export GOTESTFLAGS ?= -race
ifeq ($(detected_OS),Darwin) # see https://github.com/golang/go/issues/61229#issuecomment-1988965927, there are too many useless linker warnings that cant be fixed on Go side, waiting for Apple fix
test: GOTESTFLAGS += -ldflags=-linkmode=internal
endif
test: export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?= 5m
test: export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?= 5m
test: manifests generate-deep-copy envtest gotestsum ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GOTESTSUM) --format testdox --format-hide-empty-pkg  --format-icons hivis -- $(GOTESTFLAGS) "$(CURRENT_DIR)/..."

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-chainsaw-tests
lint-chainsaw-tests: chainsaw
	@CHAINSAW=$(CHAINSAW) ./hack/lint-chainsaw.sh

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: fmt
fmt: golangci-lint
	$(GOLANGCI_LINT) run --fix --enable-only gci,gofumpt "$(CURRENT_DIR)/..."

GO_MODULE = $(shell go list -m)
API_DIRS = $(shell find apis -mindepth 2 -type d | sed "s|^|$(shell go list -m)/|" | xargs)
.PHONY: k8s-client-gen
k8s-client-gen: applyconfiguration-gen
	@echo ">> generating internal/client/applyconfiguration..."
	@$(APPLYCONFIGURATION_GEN) \
		--output-dir "internal/client/applyconfiguration" \
		--output-pkg "$(GO_MODULE)/internal/client/applyconfiguration" \
		$(API_DIRS)

.PHONY: k8s-gvk-gen
k8s-gvk-gen:
	@echo ">> Generating generate.gvk.go"
	@go run ./cmd/gvk-gen $(API_DIRS)

##@ Build

.PHONY: build
build: generate-deep-copy
	@for dir in ./cmd/*; do \
		if [ -d "$$dir" ]; then \
			bin_name=$$(basename "$$dir"); \
			echo "Building $$bin_name..."; \
			go build -o "./bin/$$bin_name" "$$dir"; \
		fi \
	done

.PHONY: container-build
container-build: $(KO) ## Build docker image with the manager.
	KO_DOCKER_REPO=$(KO_DOCKER_REPO) $(KO) build ./cmd/operator -B --sbom none

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(CURRENT_DIR)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
APPLYCONFIGURATION_GEN ?= $(LOCALBIN)/applyconfiguration-gen
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
KO = $(LOCALBIN)/ko
GOTESTSUM = $(LOCALBIN)/gotestsum
CHAINSAW = $(LOCALBIN)/chainsaw

## Tool Versions

# renovate: datasource=github-releases depName=kubernetes-sigs/controller-tools
CONTROLLER_TOOLS_VERSION ?= v0.17.0
ENVTEST_VERSION ?= release-0.19
# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v1.63.4
# renovate: datasource=github-releases depName=ko-build/ko
KO_VERSION ?= v0.17.1
# renovate: datasource=github-releases depName=gotestyourself/gotestsum
GOTESTSUM_VERSION ?= v1.12.0
# renovate: datasource=go depName=github.com/kubernetes/code-generator
CODE_GENERATOR_VERSION ?= v0.32.0
# renovate: datasource=go depName=github.com/kyverno/chainsaw
CHAINSAW_VERSION ?= v0.2.12

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
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

.PHONY: ko
ko: $(KO)
$(KO): $(LOCALBIN)
	$(call go-install-tool,$(KO),github.com/google/ko,$(KO_VERSION))

.PHONY: chainsaw
chainsaw: $(CHAINSAW)
$(CHAINSAW): $(LOCALBIN)
	$(call go-install-tool,$(CHAINSAW),github.com/kyverno/chainsaw,$(CHAINSAW_VERSION))

.PHONY: gotestsum
gotestsum: $(GOTESTSUM)
$(GOTESTSUM): $(LOCALBIN)
	$(call go-install-tool,$(GOTESTSUM),gotest.tools/gotestsum,$(GOTESTSUM_VERSION))

.PHONY: applyconfiguration-gen
applyconfiguration-gen: $(APPLYCONFIGURATION_GEN) ## Download applyconfiguration-gen locally if necessary.
$(APPLYCONFIGURATION_GEN): $(LOCALBIN)
	$(call go-install-tool,$(APPLYCONFIGURATION_GEN),k8s.io/code-generator/cmd/applyconfiguration-gen,$(CODE_GENERATOR_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
