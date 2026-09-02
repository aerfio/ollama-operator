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
generate: manifests generate-deep-copy

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) crd applyconfiguration paths="$(CURRENT_DIR)/..." output:crd:artifacts:config="$(CURRENT_DIR)/helm/chart/ollama-operator/templates/crds"

.PHONY: generate-deep-copy
generate-deep-copy: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object paths="$(CURRENT_DIR)/..."

.PHONY: test
test: export GOTESTFLAGS ?= -race
test: export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?= 5m
test: export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?= 5m
test: manifests generate-deep-copy setup-envtest gotestsum ## Run tests.
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
	$(GOLANGCI_LINT) fmt

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

# gomodver returns the version of a Go module, honoring any replace directive.
# It is defined before ENVTEST_VERSION/ENVTEST_K8S_VERSION (which use it) to avoid
# empty values when make evaluates them during parsing.
define gomodver
$(shell go list -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' $(1) 2>/dev/null)
endef

## Location to install dependencies to
LOCALBIN ?= $(CURRENT_DIR)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen-$(CONTROLLER_TOOLS_VERSION)
ENVTEST ?= $(LOCALBIN)/setup-envtest-$(ENVTEST_VERSION)
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint-$(GOLANGCI_LINT_VERSION)
KO = $(LOCALBIN)/ko-$(KO_VERSION)
GOTESTSUM = $(LOCALBIN)/gotestsum-$(GOTESTSUM_VERSION)
CHAINSAW = $(LOCALBIN)/chainsaw-$(CHAINSAW_VERSION)

## Tool Versions

# renovate: datasource=github-releases depName=kubernetes-sigs/controller-tools
CONTROLLER_TOOLS_VERSION ?= v0.22.0
#ENVTEST_VERSION is the controller-runtime version to use for setup-envtest, derived from go.mod
ENVTEST_VERSION ?= $(shell v='$(call gomodver,sigs.k8s.io/controller-runtime)'; \
	[ -n "$$v" ] || { echo "Set ENVTEST_VERSION manually (controller-runtime replace has no tag)" >&2; exit 1; }; \
	printf '%s\n' "$$v")

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell v='$(call gomodver,k8s.io/api)'; \
	[ -n "$$v" ] || { echo "Set ENVTEST_K8S_VERSION manually (k8s.io/api replace has no tag)" >&2; exit 1; }; \
	printf '%s\n' "$$v" | sed -E 's/^v?[0-9]+\.([0-9]+).*/1.\1/')
# renovate: datasource=github-releases depName=golangci/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.13.2
# renovate: datasource=github-releases depName=ko-build/ko
KO_VERSION ?= v0.19.1
# renovate: datasource=github-releases depName=gotestyourself/gotestsum
GOTESTSUM_VERSION ?= v1.13.0
# renovate: datasource=go depName=github.com/kyverno/chainsaw
CHAINSAW_VERSION ?= v0.2.15

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@"$(ENVTEST)" use $(ENVTEST_K8S_VERSION) --bin-dir "$(LOCALBIN)" -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))


.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

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

# go-install-tool will 'go install' any package with custom target and name of binary.
# The binary is installed to the versioned target path, so bumping the tool version
# (e.g. CONTROLLER_TOOLS_VERSION) causes make to install the new version.
# $1 - target path with name of binary (including the version)
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f "$(1)" "$(LOCALBIN)/$$(basename "$(2)")" ;\
GOBIN="$(LOCALBIN)" go install $${package} ;\
mv "$(LOCALBIN)/$$(basename "$(2)")" "$(1)"
endef
