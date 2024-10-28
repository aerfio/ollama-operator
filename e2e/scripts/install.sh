#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o errtrace

REPO_ROOT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )/../.." &> /dev/null && pwd )"

HELM="${HELM:-helm}"

if [ -z "${1+x}" ] || [ -z "$1" ]; then
    echo "Error: provide container image tag as first argument to use to install ollama-operator helm chart"
    exit 1
fi

"$HELM" upgrade -i ollama-operator "$REPO_ROOT_DIR/helm/chart/ollama-operator" \
--namespace ollama-operator \
--create-namespace \
--set-json additionalOperatorArgs='["-v=3"]' \
--set "image.tag=$1" \
--atomic
