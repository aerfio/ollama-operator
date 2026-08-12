# AGENTS.md

Kubernetes operator that reconciles `Model` and `Prompt` custom resources against the Ollama HTTP API. Module `aerf.io/ollama-operator`, Go 1.26.

## Commands

- `make generate` — regenerate code. Two steps: `manifests` (`controller-gen crd applyconfiguration`) and `generate-deep-copy` (`controller-gen object`). Output: `apis/ollama/v1alpha1/applyconfiguration/`, `zz_generated.deepcopy.go`, and CRDs under `helm/chart/ollama-operator/templates/crds/`. Never hand-edit those files.
- `make test` — runs all packages with `-race` under envtest (auto-downloads K8s control-plane binaries via `setup-envtest`).
- Focused test: `KUBEBUILDER_ASSETS="$(./bin/setup-envtest-v0.24.1 use 1.36 --bin-dir bin -p path)" go test ./internal/controllers/model/...`
- `make lint` / `make lint-fix` / `make fmt` — golangci-lint v2 (`.golangci.yml` uses `default: none` + explicit enables).
- `make build` — builds both binaries to `./bin/`: `cmd/operator` (the controller manager) and `cmd/mergepatcher` (CLI that dry-runs merge-patch of a Model CR, see `samples/mergepatcher/`).
- `make lint-chainsaw-tests` — lints e2e scenarios in `e2e/scenarios/` (needs the `chainsaw` binary).
- e2e requires a real kind cluster + built image; CI does this (`ci.yaml`). Local trace stack: `hack/run-otel-lgtm.sh`.

CI also runs `go mod tidy -diff` and `git diff --exit-code` after `make test`/`make build`, so keep go.mod tidy and commit regenerated output.

## Go codebase exploration

- Prefer the **gopls MCP server** (configured in `opencode.json`) and **`go doc <pkg>.<sym>`** to explore Go code (including dependencies). Do **not** read files directly in `GOMODCACHE` — use `go doc` and gopls instead.

## Architecture

- `apis/ollama/v1alpha1/` — `Model` and `Prompt` types + generated applyconfiguration/deepcopy. `gvk.go` defines `ModelGroupVersionKind`/`PromptGroupVersionKind`.
- `internal/controllers/model/` — per-Model StatefulSet + Service (owned resources), pulls the model via Ollama API.
- `internal/controllers/prompt/` — runs prompts against the referenced Model; watches Models to trigger reconciliation.
- `internal/ollamaclient/` — Ollama HTTP client provider (`mocks.go` for tests).
- `internal/patches/`, `internal/eventrecorder/`, `internal/restconfig/`, `internal/commonmeta/`, `internal/defaults/`, `internal/k8serrors/`, `internal/applyconfig/`.
- `helm/chart/ollama-operator/` — installable chart; CRDs are generated there and committed.

## Gotchas

- Crossplane common API types (conditions, `SecretKeySelector`, etc.) come from `github.com/crossplane/crossplane/apis/v2/core/v2` (imported as `xpv2` or `v2`), **not** from `crossplane-runtime` — it dropped `apis/common` in v2.3. This is a Renovate-managed dependency.
- `apis/ollama/v1alpha1/copied_condition_crossplane.go` is a hand-copied `ConditionedStatus` from crossplane-runtime, needed because applyconfiguration-gen crashes on the original types (comment in the file explains the bug). Do not delete or refactor it.
- Makefile tools are installed to **versioned filenames** (`bin/controller-gen-v0.21.0`). Bump a tool by changing its `*_VERSION` var; `make` reinstalls automatically. `ENVTEST_VERSION`/`ENVTEST_K8S_VERSION` derive from go.mod via `gomodver` (bump `sigs.k8s.io/controller-runtime`/`k8s.io/api` to change them) — do not set them manually.
- Tool versions must stay in sync across `Makefile`, `.github/workflows/ci.yaml`, and `.github/renovate.json5`; `hack/check-renovate-managers.mjs` enforces this. Renovate post-upgrade runs `make generate` and `hack/update-kind-node-image.sh` (keeps `KIND_NODE_IMAGE` in sync with `KIND_VERSION`).
- Model controller tests use envtest and load CRDs from the helm chart dir via `internal/testutils` (`GetCRDsDir`). `internal/testutils/cmp.go` provides `IgnoreXPv1ConditionFields` for comparing conditions.
