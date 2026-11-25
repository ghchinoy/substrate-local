# `substrate-local` — Agent Instructions

Standalone zero-Kubernetes Agent Substrate `v1.0.0` Control API + `atenet-router` daemon backed by macOS Apple `container` (`Virtualization.framework`) and rootless `podman`.

## Hard Rules

- **Zero `github.com/google/ax` Go module dependency:** `pkg/sublocal` and `cmd/substrate-local` must depend only on `go.ate.dev/substrate` (`v1.0.0`), gRPC/Protobuf, YAML, and Go stdlib so upstream `google/ax` churn never breaks this module.
- **`pkg/sublocal` is the public Go package** for embedding the local Substrate Control + Router server in Go tools and tests.
- **License header:** Every `.go`, `.sh`, and `Makefile` file carries the Apache-2.0 header.

## Layout

- `cmd/substrate-local/` — CLI daemon entrypoint (`--addr`, `--runtime`, `--config`, `--snapshot-dir`, `--version` / `version`)
- `internal/version/` — Build-time version/commit/date stamping via `-ldflags`
- `pkg/sublocal/` — Public `Server`, `Config`, `ContainerRunner` (`apple` & `podman`), unit + live Apple `container` tests
- `scripts/ax-stack.sh` — Single-cluster (`up`/`down`) and dual-cluster (`multi-up`/`migrate`) local AX stack orchestrator pairing `substrate-local` with unmodified upstream `google/ax` (`AX_VERSION` clone-free module fetch or local `AX_REPO`)
- `scripts/test-ax-compat.sh` — End-to-end contract regression suite (`make test-compat`) validating `substrate-local` against unmodified upstream `google/ax`
- `scripts/check-substrate-drift.sh` — Checks `go.ate.dev/substrate` & `google/ax` release drift and verifies `ControlServer` RPC coverage (`make check-drift`)
- `examples/` — Sample `substrate-local.yaml` and `task-local.yaml`
- `docs/quickstart-apple-container.md` — macOS Apple `container` + AX quickstart
- `docs/quickstart-podman.md` — Rootless Podman + AX quickstart (macOS & Linux)
- `docs/choosing-a-runtime.md` — Compatibility boundary & decision guide (`substrate-local` vs. Kind vs. GKE)
- `docs/architecture.md` — Architecture, runtime comparison, and limitations
- `docs/diagrams/` — Semantic Graphviz `.dot` sources and dual-theme (`-light.webp` / `-dark.webp`) diagrams rendered via `make diagrams` (`../graphviz-style/theme/lustrous.gvpr`)

## Verification

```bash
export GOTOOLCHAIN=auto
go mod tidy
go fmt ./...
go vet ./...
go build ./...
go test -race -v ./pkg/sublocal/...
make test-compat                 # fresh upstream google/ax regression (set TEST_PODMAN=1 for dual-runtime mobility)
```
