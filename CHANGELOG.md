# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and [Common Changelog](https://common-changelog.org/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

_Initial standalone release preparation._

### Added

- Standalone zero-Kubernetes Agent Substrate `v1.0.0` Control API (`ateapi/v1.ControlServer`) and embedded HTTP/1.1 + HTTP/2 `h2c` `atenet-router` (`ate-target-actor` wake-on-request proxy) exported as public Go package `pkg/sublocal` with zero `github.com/google/ax` dependency (`cmd/substrate-local`, `pkg/sublocal`).
- Dual container runtime support for macOS native Apple `container` (`Virtualization.framework`, direct `vmnet` networking) and rootless Podman (`127.0.0.1:<port>` publishing), including `--entrypoint` parity with Kubernetes/Substrate `Container.command`, `/run/ate/actor-uid` projection, `/workspace` `.tar.gz` snapshots across `SuspendActor`/`ResumeActor`, and immutable `Tag` fan-out.
- Single-cluster (`make up`/`down`) and dual-cluster (`make multi-up`/`migrate`) local AX stack orchestration (`scripts/ax-stack.sh`) with clone-free `AX_VERSION` Go module fetching (defaulting to `google/ax` `v0.4.0-rc.2`) alongside local `AX_REPO` support [`substrate-local-0nb`].
- End-to-end upstream `google/ax` contract regression suite (`scripts/test-ax-compat.sh` / `make test-compat`) validating `hack/tour.sh` conformance, PID-1/plain-image runner injection, `>1 MiB` `h2c` `ax ssh` streaming, and optional dual-runtime Apple `container` ↔ rootless Podman mobility + `ax promote` [`substrate-local-hky`].
- Build-time version/commit/date stamping (`substrate-local --version` and `substrate-local version`), GitHub Actions CI (`.github/workflows/ci.yml`), and tag-triggered multi-platform release workflow (`.github/workflows/release.yml` for `darwin`/`linux` `arm64`/`amd64` + `checksums.txt`) [`substrate-local-7kc`, `substrate-local-an3`].
- Dual-theme (`light`/`dark` WebP) Lustrous Graphviz architecture, lifecycle/snapshot, and dual-cluster mobility diagrams (`docs/diagrams/`, `make diagrams`) embedded in `README.md` and `docs/architecture.md` [`substrate-local-0rq`].
- Upstream drift tracking via `TestControlServerCoverage` (reflection-checked `ateapi/v1.ControlServer` RPC allowlist) and `scripts/check-substrate-drift.sh` (`make check-drift` + weekly `.github/workflows/drift.yml`) comparing pinned `go.ate.dev/substrate` and `google/ax` releases [`substrate-local-2mm`].

[Unreleased]: https://github.com/ghchinoy/substrate-local/commits/main
