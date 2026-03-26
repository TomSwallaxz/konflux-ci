#!/usr/bin/env bash
# Copy CLI binaries from task-runner into /mnt/e2e-shared/bin for go-toolset steps
# (go-toolset has go/make but not kubectl, yq, or jq).
#
# Clone / fetch-kubeconfig run as root; go-toolset runs non-root. Without fixing modes,
# apply-overrides and other writers get "permission denied" on root-owned files under the repo.
set -euo pipefail

DEST="${TEKTON_SHARED_BIN:-/mnt/e2e-shared/bin}"
mkdir -p "${DEST}"
cp -a /usr/local/bin/kubectl /usr/local/bin/yq /usr/bin/jq "${DEST}/"
chmod a+rx "${DEST}/kubectl" "${DEST}/yq" "${DEST}/jq"

REPO_ROOT="${TEKTON_REPO_ROOT:-/mnt/konflux-ci/repo}"
if [[ -d "${REPO_ROOT}" ]]; then
  chmod -R a+rwX "${REPO_ROOT}"
fi
