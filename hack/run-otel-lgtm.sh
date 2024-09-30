#!/usr/bin/env bash

set -eu

RELEASE=${1:-latest}

docker run \
  --name lgtm \
  -p 3000:3000 \
  -p 4317:4317 \
  -p 4318:4318 \
  --rm \
  -ti \
  docker.io/grafana/otel-lgtm:${RELEASE}
