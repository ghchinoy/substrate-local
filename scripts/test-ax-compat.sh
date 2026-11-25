#!/usr/bin/env bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# test-ax-compat.sh runs a persistent end-to-end compatibility regression suite
# validating substrate-local against an unmodified upstream google/ax checkout.
#
# Usage:
#   ./scripts/test-ax-compat.sh
#   TEST_PODMAN=1 ./scripts/test-ax-compat.sh
#
# Environment:
#   AX_REPO        Path to unmodified google/ax checkout (default: ~/projects/ax)
#   AX_TASK_IMAGE  Digest-pinned ax-runner image (default: v0.4.0-rc.2 public digest)
#   PLAIN_IMAGE    Digest-pinned plain image without ax-runner (default: python:3.12-slim)
#   FRESH_STACK    1 to rebuild and restart the local stack first (default: 1)
#   TEST_PODMAN    1 to also run Apple container <-> rootless Podman mobility + ax promote (default: 0)
#   TOUR_TIMEOUT   Seconds to wait per task state transition in tour.sh (default: 60)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

AX_REPO="${AX_REPO:-${HOME}/projects/ax}"
AX_TASK_IMAGE="${AX_TASK_IMAGE:-ghcr.io/google/ax/ax-runner@sha256:0937ebe5ad2d8db6bb25e88ded44d2d65de1b1bd8f0a9a97c6bb3787407d9947}"
PLAIN_IMAGE="${PLAIN_IMAGE:-python:3.12-slim@sha256:5e2bdb1b0d542d5039cffb8181c65930093949194a65c365dfb1b0fe13a4fa5b}"
FRESH_STACK="${FRESH_STACK:-1}"
TEST_PODMAN="${TEST_PODMAN:-0}"
TOUR_TIMEOUT="${TOUR_TIMEOUT:-60}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/substrate-local-compat-XXXXXX")"
cleanup() {
  rm -rf "${TMP_DIR}"
  if [[ -f "${REPO_ROOT}/.substrate-local/env" ]]; then
    # shellcheck disable=SC1091
    source "${REPO_ROOT}/.substrate-local/env"
    for t in compat-pid1 compat-plain compat-ssh compat-mobility compat-promote; do
      ax delete task "${t}" >/dev/null 2>&1 || true
      if [[ -n "${AX_SERVER_B:-}" ]]; then
        ax --server "${AX_SERVER_B}" delete task "${t}" >/dev/null 2>&1 || true
      fi
    done
  fi
}
trap cleanup EXIT

step() { printf '\n==> %s\n' "$*"; }
pass() { printf '  PASS: %s\n' "$*"; }
fail() { printf '  FAIL: %s\n' "$*" >&2; exit 1; }

[[ -d "${AX_REPO}/cmd/ax-server" ]] || fail "AX_REPO='${AX_REPO}' is not a google/ax checkout"
AX_SHA="$(git -C "${AX_REPO}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
step "Validating substrate-local against upstream google/ax (${AX_REPO} @ ${AX_SHA})"

if [[ "${FRESH_STACK}" == "1" ]]; then
  step "0. Rebuilding and starting fresh local stack"
  ./scripts/ax-stack.sh down --keep-postgres >/dev/null 2>&1 || true
  ./scripts/ax-stack.sh up
fi

# shellcheck disable=SC1091
source "${REPO_ROOT}/.substrate-local/env"
AX_BIN="${REPO_ROOT}/.substrate-local/bin/ax"
[[ -x "${AX_BIN}" ]] || fail "ax binary not found at ${AX_BIN}"
pass "ax CLI: $("${AX_BIN}" version)"

# ---------------------------------------------------------------------------
# 1. Upstream hack/tour.sh conformance (skipping only atenet-egress checks)
# ---------------------------------------------------------------------------
step "1. Upstream hack/tour.sh conformance (${AX_TASK_IMAGE})"
[[ -f "${AX_REPO}/hack/tour.sh" ]] || fail "${AX_REPO}/hack/tour.sh not found"
TOUR_SCRIPT="${TMP_DIR}/tour-substrate-local.sh"
sed -e 's/^expect_fail ax ssh "${SIMPLE_TASK}" -- git ls-remote/# [substrate-local: no atenet-egress] skip/' \
    -e 's/^expect_fail ax ssh "${AGENT}" -- curl/# [substrate-local: no atenet-egress] skip/' \
  "${AX_REPO}/hack/tour.sh" > "${TOUR_SCRIPT}"
chmod +x "${TOUR_SCRIPT}"

(
  unset GEMINI_API_KEY
  export AX_BIN AX_TASK_IMAGE TOUR_TIMEOUT NO_COLOR=1
  "${TOUR_SCRIPT}"
)
pass "Upstream hack/tour.sh completed (10/10 non-K8s steps)"

# ---------------------------------------------------------------------------
# 2. PID 1 (--entrypoint) and plain-image runner injection
# ---------------------------------------------------------------------------
step "2. Verifying PID 1 (--entrypoint) and plain-image runner injection"
ax delete task compat-pid1 >/dev/null 2>&1 || true
ax apply -f - <<EOF
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: compat-pid1
spec:
  profile:
    image: "${AX_TASK_IMAGE}"
  commands: ["sleep", "300"]
  debug: true
EOF
ax resume task compat-pid1 >/dev/null
PID1_CMD="$(ax ssh compat-pid1 -- ps -o args= -p 1 | tr -s ' ' | sed 's/^ //;s/ $//')"
[[ "${PID1_CMD}" == "/usr/local/bin/ax-runner" ]] || fail "expected PID 1 '/usr/local/bin/ax-runner', got '${PID1_CMD}'"
ax delete task compat-pid1 >/dev/null
pass "PID 1 on ax-runner image is cleanly '${PID1_CMD}' (no duplicated argv)"

ax delete task compat-plain >/dev/null 2>&1 || true
ax apply -f - <<EOF
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: compat-plain
spec:
  profile:
    image: "${PLAIN_IMAGE}"
  commands: ["sleep", "300"]
  debug: true
EOF
ax resume task compat-plain >/dev/null
PLAIN_OUT="$(ax ssh compat-plain -- python3 -c 'print(2+2)' | tr -d '\r\n')"
[[ "${PLAIN_OUT}" == "4" ]] || fail "plain python:3.12-slim exec returned '${PLAIN_OUT}', want '4'"
ax delete task compat-plain >/dev/null
pass "Plain image (${PLAIN_IMAGE}) booted with injected ax-runner and executed python3"

# ---------------------------------------------------------------------------
# 3. ax ssh h2c streaming (>1 MiB) and 10x rapid exec stress
# ---------------------------------------------------------------------------
step "3. Verifying ax ssh h2c streaming (>1 MiB) and 10x rapid exec"
ax delete task compat-ssh >/dev/null 2>&1 || true
ax apply -f - <<EOF
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: compat-ssh
spec:
  profile:
    image: "${AX_TASK_IMAGE}"
  commands: ["sleep", "300"]
  debug: true
EOF
ax resume task compat-ssh >/dev/null
BYTES="$(ax ssh compat-ssh -- python3 -c 'import sys; sys.stdout.write("A" * (1024*1024 + 123))' | wc -c | tr -d ' ')"
[[ "${BYTES}" == "1048699" ]] || fail "streamed ${BYTES} bytes over ax ssh, want 1048699"
for i in $(seq 1 10); do
  OUT="$(ax ssh compat-ssh -- echo "ping-${i}" | tr -d '\r\n')"
  [[ "${OUT}" == "ping-${i}" ]] || fail "rapid ax ssh #${i} returned '${OUT}', want 'ping-${i}'"
done
ax delete task compat-ssh >/dev/null
pass "Streamed ${BYTES} bytes and passed 10/10 rapid ax ssh calls over h2c router"

# ---------------------------------------------------------------------------
# 4. Optional dual-runtime mobility (Apple container <-> rootless Podman)
# ---------------------------------------------------------------------------
if [[ "${TEST_PODMAN}" == "1" ]]; then
  step "4. Dual-runtime Apple container <-> rootless Podman mobility & ax promote"
  command -v podman >/dev/null 2>&1 || fail "TEST_PODMAN=1 requires podman in PATH"
  podman info >/dev/null 2>&1 || fail "TEST_PODMAN=1 requires a running podman machine (run 'podman machine start')"

  ./scripts/ax-stack.sh down --keep-postgres
  AX_LOCAL_CLUSTER_B_RUNTIME=podman ./scripts/ax-stack.sh multi-up
  # shellcheck disable=SC1091
  source "${REPO_ROOT}/.substrate-local/env"

  # 4a. migrate with /workspace preservation
  ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_A}" ax --server "${AX_SERVER_A}" apply -f - <<EOF
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: compat-mobility
spec:
  profile:
    image: "debian:12"
  commands: ["sleep", "300"]
  debug: true
EOF
  ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_A}" ax --server "${AX_SERVER_A}" resume task compat-mobility >/dev/null
  ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_A}" ax --server "${AX_SERVER_A}" ssh compat-mobility -- sh -c 'echo "compat-apple-to-podman" > /workspace/proof.txt'
  ./scripts/ax-stack.sh migrate compat-mobility >/dev/null
  MIG_OUT="$(ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_B}" ax --server "${AX_SERVER_B}" ssh compat-mobility -- cat /workspace/proof.txt | tr -d '\r\n')"
  [[ "${MIG_OUT}" == "compat-apple-to-podman" ]] || fail "migrated /workspace/proof.txt='${MIG_OUT}', want 'compat-apple-to-podman'"
  pass "Migrated compat-mobility from Apple container to rootless Podman preserving /workspace"

  # 4b. upstream ax promote --suspend-source
  ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_A}" ax --server "${AX_SERVER_A}" apply -f - <<EOF
apiVersion: ax.io/v1alpha1
kind: Task
metadata:
  name: compat-promote
spec:
  profile:
    image: "debian:12"
  commands: ["sleep", "300"]
  debug: true
EOF
  ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_A}" ax --server "${AX_SERVER_A}" resume task compat-promote >/dev/null
  ax --server "${AX_SERVER_A}" promote task compat-promote --to-server "${AX_SERVER_B}" --suspend-source >/dev/null
  PROM_OUT="$(ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_B}" ax --server "${AX_SERVER_B}" ssh compat-promote -- uname -s | tr -d '\r\n')"
  [[ "${PROM_OUT}" == "Linux" ]] || fail "promoted task ax ssh returned '${PROM_OUT}', want 'Linux'"
  pass "Upstream ax promote --suspend-source succeeded (Apple container -> rootless Podman)"

  for t in compat-mobility compat-promote; do
    ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_A}" ax --server "${AX_SERVER_A}" delete task "${t}" >/dev/null 2>&1 || true
    ATENET_ROUTER_ADDR="${ATENET_ROUTER_ADDR_B}" ax --server "${AX_SERVER_B}" delete task "${t}" >/dev/null 2>&1 || true
  done
  ./scripts/ax-stack.sh multi-down --keep-postgres
  ./scripts/ax-stack.sh up >/dev/null
fi

# ---------------------------------------------------------------------------
# 5. Clean teardown verification
# ---------------------------------------------------------------------------
step "5. Verifying clean teardown"
LEFTOVER_CONTAINERS="$(container list --all 2>/dev/null | awk 'NR>1 && $1 ~ /^ate-.*(compat-|tour-)/ {print $1}' || true)"
[[ -z "${LEFTOVER_CONTAINERS}" ]] || fail "leftover test containers found: ${LEFTOVER_CONTAINERS}"
pass "Zero leftover compat/tour actor containers"

printf '\nAll substrate-local <-> google/ax (%s) compatibility tests PASSED.\n' "${AX_SHA}"
