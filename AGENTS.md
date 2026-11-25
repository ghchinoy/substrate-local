# `substrate-local` — Agent Instructions

Standalone zero-Kubernetes Agent Substrate `v1.0.0` Control API + `atenet-router` daemon backed by macOS Apple `container` (`Virtualization.framework`) and rootless `podman`.

## Hard Rules

- **Zero `github.com/google/ax` Go module dependency:** `pkg/sublocal` and `cmd/substrate-local` must depend only on `go.ate.dev/substrate` (`v1.0.0`), gRPC/Protobuf, YAML, and Go stdlib so upstream `google/ax` churn never breaks this module.
- **`pkg/sublocal` is the public Go package** for embedding the local Substrate Control + Router server in Go tools and tests.
- **License header:** Every `.go`, `.sh`, and `Makefile` file carries the Apache-2.0 header.

## Layout

- `cmd/substrate-local/` — CLI daemon entrypoint (`--addr`, `--runtime`, `--config`, `--snapshot-dir`)
- `pkg/sublocal/` — Public `Server`, `Config`, `ContainerRunner` (`apple` & `podman`), unit + live Apple `container` tests
- `scripts/ax-stack.sh` — Single-cluster (`up`/`down`) and dual-cluster (`multi-up`/`migrate`) local AX stack orchestrator pairing `substrate-local` with unmodified upstream `google/ax` (`AX_REPO`, default `~/projects/ax`)
- `scripts/test-ax-compat.sh` — End-to-end contract regression suite (`make test-compat`) validating `substrate-local` against unmodified upstream `google/ax`
- `examples/` — Sample `substrate-local.yaml` and `task-local.yaml`
- `docs/architecture.md` — Architecture, runtime comparison, and limitations

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
