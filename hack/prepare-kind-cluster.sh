#!/usr/bin/env bash

set -euo

kind create cluster --wait=5m # todo turn on tracing on apiserver and other k8s components
helm upgrade -i grafana-operator oci://ghcr.io/grafana/helm-charts/grafana-operator --version v5.13.0 -n grafana-operator --create-namespace --atomic
helm repo add grafana https://grafana.github.io/helm-charts
helm install tempo grafana/tempo --version 1.10.3 --create-namespace --namespace tempo --atomic

# Create multiple YAML objects from stdin
kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: grafana
spec: {}
---
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDatasource
metadata:
  name: tempo
  namespace: "grafana"
spec:
  allowCrossNamespaceImport: true
  datasource:
    access: proxy
    name: Traces
    type: tempo
    uid: traces
    url: http://tempo.tempo:3100
  instanceSelector:
    matchLabels:
      traces: grafana
---
apiVersion: grafana.integreatly.org/v1beta1
kind: Grafana
metadata:
  name: grafana
  namespace: grafana
  labels:
    traces: "grafana"
spec:
  config:
    log:
      mode: "console"
    auth:
      disable_login_form: "false"
    security:
      admin_user: admin
      admin_password: admin
EOF
