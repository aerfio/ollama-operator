#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o errtrace

CHAINSAW="${CHAINSAW:-chainsaw}"
REPO_ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/.." &> /dev/null && pwd )"

find "${REPO_ROOT_DIR}/e2e/scenarios" -name chainsaw-test.yaml -exec echo "Linting {}" \; -exec "$CHAINSAW" lint test -f {} \;
find "${REPO_ROOT_DIR}/e2e" -name ".chainsaw.yaml" -exec echo "Linting {}" \; -exec "$CHAINSAW" lint configuration -f {} \;
