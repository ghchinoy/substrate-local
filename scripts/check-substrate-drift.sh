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

# check-substrate-drift.sh compares pinned upstream versions in substrate-local
# against the latest Agent Substrate and AX releases and verifies ControlServer
# RPC coverage via TestControlServerCoverage.
#
# Usage:
#   ./scripts/check-substrate-drift.sh [--strict] [--markdown]
#
# Flags:
#   --strict    Exit non-zero if a newer upstream Substrate or AX release exists
#   --markdown  Print a Markdown summary table (suitable for GITHUB_STEP_SUMMARY)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${REPO_ROOT}"

STRICT=0
MARKDOWN=0
for arg in "$@"; do
  case "${arg}" in
    --strict) STRICT=1 ;;
    --markdown) MARKDOWN=1 ;;
    *) echo "usage: $0 [--strict] [--markdown]" >&2; exit 2 ;;
  esac
done

latest_gh_release() {
  local repo="$1"
  local tag=""
  if command -v gh >/dev/null 2>&1; then
    tag="$(gh api "repos/${repo}/releases?per_page=1" --jq '.[0].tag_name // empty' 2>/dev/null || true)"
  fi
  if [[ -z "${tag}" ]] && command -v curl >/dev/null 2>&1; then
    tag="$(curl -fsSL "https://api.github.com/repos/${repo}/releases?per_page=1" 2>/dev/null \
      | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1 || true)"
  fi
  printf '%s' "${tag:-unknown}"
}

PINNED_SUBSTRATE="$(GOWORK=off go list -m -f '{{.Version}}' go.ate.dev/substrate)"
LATEST_SUBSTRATE_MOD="$(GOWORK=off go list -m -versions -f '{{range .Versions}}{{println .}}{{end}}' go.ate.dev/substrate 2>/dev/null | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | tail -n1 || true)"
LATEST_SUBSTRATE_GH="$(latest_gh_release agent-substrate/substrate)"
LATEST_SUBSTRATE="${LATEST_SUBSTRATE_MOD:-${LATEST_SUBSTRATE_GH}}"
[[ -n "${LATEST_SUBSTRATE}" ]] || LATEST_SUBSTRATE="unknown"

PINNED_AX="$(sed -n 's/^DEFAULT_AX_VERSION="\([^"]*\)".*/\1/p' scripts/ax-stack.sh | head -n1)"
LATEST_AX="$(latest_gh_release google/ax)"

COVERAGE_OUT="$(GOWORK=off go test -v -count=1 ./pkg/sublocal -run TestControlServerCoverage 2>&1)"
COVERAGE_LINE="$(printf '%s\n' "${COVERAGE_OUT}" | sed -n 's/.*\(ateapi\/v1\.ControlServer coverage:.*\)/\1/p' | head -n1)"

SUBSTRATE_STATUS="up-to-date"
DRIFT_FOUND=0
if [[ "${LATEST_SUBSTRATE}" != "unknown" && "${PINNED_SUBSTRATE}" != "${LATEST_SUBSTRATE}" ]]; then
  SUBSTRATE_STATUS="update available (${PINNED_SUBSTRATE} -> ${LATEST_SUBSTRATE})"
  DRIFT_FOUND=1
fi

AX_STATUS="up-to-date"
if [[ "${LATEST_AX}" != "unknown" && "${PINNED_AX}" != "${LATEST_AX}" ]]; then
  AX_STATUS="update available (${PINNED_AX} -> ${LATEST_AX})"
  DRIFT_FOUND=1
fi

if [[ "${MARKDOWN}" == "1" ]]; then
  cat <<EOF
## Upstream Drift Report

| Component | Pinned | Latest Upstream | Status |
|---|---|---|---|
| \`go.ate.dev/substrate\` | \`${PINNED_SUBSTRATE}\` | \`${LATEST_SUBSTRATE}\` | ${SUBSTRATE_STATUS} |
| \`google/ax\` (\`DEFAULT_AX_VERSION\`) | \`${PINNED_AX}\` | \`${LATEST_AX}\` | ${AX_STATUS} |

- **RPC coverage:** ${COVERAGE_LINE:-unknown}
EOF
else
  printf 'go.ate.dev/substrate : pinned=%-14s latest=%-14s [%s]\n' "${PINNED_SUBSTRATE}" "${LATEST_SUBSTRATE}" "${SUBSTRATE_STATUS}"
  printf 'google/ax (default)  : pinned=%-14s latest=%-14s [%s]\n' "${PINNED_AX}" "${LATEST_AX}" "${AX_STATUS}"
  printf 'ControlServer RPCs   : %s\n' "${COVERAGE_LINE:-unknown}"
fi

if [[ "${STRICT}" == "1" && "${DRIFT_FOUND}" == "1" ]]; then
  exit 1
fi
