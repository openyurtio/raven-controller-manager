#!/usr/bin/env bash

set -x
set -e

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"

TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}"/src/github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client
cp -r ${YURT_ROOT}/{go.mod,go.sum} "${TMP_DIR}"/src/github.com/openyurtio/raven-controller-manager/
cp -r ${YURT_ROOT}/pkg/ravencontroller/{apis,hack} "${TMP_DIR}"/src/github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/

(
  cd "${TMP_DIR}"/src/github.com/openyurtio/raven-controller-manager/;
  HOLD_GO="${TMP_DIR}/src/github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/hack/hold.go"
  printf 'package hack\nimport "k8s.io/code-generator"\n' > ${HOLD_GO}
  go mod vendor
  GOPATH=${TMP_DIR} GO111MODULE=off /bin/bash vendor/k8s.io/code-generator/generate-groups.sh all \
    github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis raven:v1alpha1 -h ./pkg/ravencontroller/hack/boilerplate.go.txt
)

rm -rf ./pkg/ravencontroller/client/{clientset,informers,listers}
mv "${TMP_DIR}"/src/github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/client/* ./pkg/ravencontroller/client

