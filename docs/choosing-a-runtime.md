# Choosing a Runtime: `substrate-local` vs. Kind (`ax local`) vs. GKE

Because [AX](https://github.com/google/ax) talks exclusively to the [Agent Substrate](https://github.com/agent-substrate/substrate) `v1.0` Control API (`ateapi/v1`) and `atenet-router`, the exact same `Task`, `Workspace`, `Model`, and `Gateway` YAML manifests run across three tiers — from a 3-second laptop microVM up to a multi-tenant Google Cloud GKE cluster.

This guide defines the **compatibility boundary** of `substrate-local` and helps you pick the right tier for what you are doing today.

---

## Quick Decision Matrix

| Capability / Need | **`substrate-local`** *(Apple `container` / Rootless Podman)* | **Kind (`ax local start`)** *(Colima / OrbStack / Docker / Rootful Podman)* | **Google Cloud GKE** *(Production Agent Substrate)* |
|---|---|---|---|
| **Fresh startup time** | **~3 seconds** | ~3–5 minutes | Always-on managed cluster |
| **Idle memory footprint** | **<512 MiB** (Postgres only; suspended actors hold 0 RAM) | **6–8 GiB** persistent Linux VM | Autoscaled `WorkerPool` nodes + GCS snapshots |
| **Docker / rootful VM required?** | **No** (native macOS `Virtualization.framework` or rootless Podman) | **Yes** (rootful engine + Linux kernel >= 6.8) | Managed GKE (`WorkerPool` nodes) |
| **10–50 agent swarm fan-out on a laptop** | **Best fit** (0-RAM suspend + wake-on-request) | Memory-bound inside single Kind VM | **Best fit** at team / production scale |
| **`Task`, `Workspace` setup, `Tag` fan-out, `ax ssh`, `ax promote`** | **Full parity** (verified via `make test-compat`) | **Full parity** | **Full parity** |
| **Default-deny L7 egress (`atenet-egress`)** | **No** (host bridge network; `Gateway` stored for validation only) | **Yes** (real Envoy `atenet-egress`) | **Yes** (production `atenet-egress` + SPIFFE) |
| **Keyless Kubernetes Secret injection (`Model` / `Gateway`)** | **No** (no Kubernetes Secrets / `atenet-egress`) | **Yes** (`ax.io/credential=true` Secrets) | **Yes** (K8s Secrets / Secret Manager) |
| **Untrusted / hostile code isolation** | Basic container / microVM boundary only | gVisor (`runsc` `systrap`) + zero-egress | Full gVisor / microVM + zero-trust network PEP |
| **`ax studio` 1-click setup UI** | CLI / `ax` / external orchestrators | **Native** (`ax studio` drives `ax local start`) | Via kubeconfig context / `ax promote` |

---

## When to Use Each Tier

### 1. Choose `substrate-local` for Fast Inner-Loop Development & Swarms
Use `substrate-local` when you want:
- **Zero-Docker, zero-rootful macOS or Linux development** (~3s startup, no 8 GiB VM draining your battery).
- **Rapid iteration on agent prompts, tools, MCP configs, `Workspace` setup, and `ax-runner` behavior** against unmodified upstream `ax` and `ax-server`.
- **Multi-agent laptop swarms (10–50 tasks)** where suspending idle actors to 0 RAM and waking them on inbound HTTP/gRPC requests makes fan-out practical on a 16–32 GiB laptop.
- **Cross-runtime promotion testing** (`make multi-up` running Apple `container` and rootless Podman side by side).

### 2. Choose Kind (`ax local start` + `ax studio`) for Full Local Parity
Switch to upstream Kind-based `ax local start` when you need to test:
- **Zero-egress enforcement** (verifying that tasks without a `Gateway` cannot reach the internet, and that `Gateway` hostname allowlists block non-matching domains).
- **Keyless API credential injection** (`kubectl create secret ...` + `ax.io/credential=true` injected by `atenet-egress` without placing API keys in the sandbox).
- **`ax studio`'s 1-click local setup UX** or Kubernetes manifest / `deploy/` changes.

### 3. Choose Google Cloud GKE for Untrusted Code, Teams & Scale
Promote your unchanged manifests (`ax promote task <name> --to <gke-context>`) to GKE + Agent Substrate when you need:
- **Strong multi-tenant & untrusted-code isolation** (gVisor/microVM workers, SPIFFE mTLS, `atenet-egress` SSRF & allowlist enforcement, RBAC `AccessPolicy`).
- **Durable GCS snapshot storage & `WorkerPool` autoscaling** across hundreds or thousands of concurrent or parked long-running sessions.
- **Shared team clusters, CI pipelines, and centralized observability** (OTLP traces, Prometheus metrics, and trajectory collection).

---

## `substrate-local` Compatibility Boundary (`go.ate.dev/substrate` v1.0.x)

`substrate-local` intentionally stays small, fast, and rootless rather than re-implementing Kubernetes. Its contract is guarded automatically by `TestControlServerCoverage` (`pkg/sublocal/drift_test.go`) and `make check-drift`:

- **In scope (maintained at parity with upstream AX):**
  - `ateapi/v1.ControlServer` core lifecycle (**24/42 RPCs**): `Atespace`, `ActorTemplate`, `Actor` (`Create`/`Get`/`List`/`Resume`/`Suspend`/`Revert`/`Delete`), `Tag` (`Create`/`Get`/`List`/`Delete` + `source_tag`), `ActorEgressPolicy` CRUD, and `ListWorkers`.
  - Embedded HTTP/1.1 + HTTP/2 `h2c` `atenet-router` (`ate-target-actor` header + wake-on-request).
  - Container `--entrypoint` parity with `Container.command`, `/run/ate/actor-uid` projection (and future `substrate#3876` per-actor config), `/workspace` `.tar.gz` volume snapshots, and read-only `ActorTemplate` OCI image volumes (`/opt/ax` via `--runner-volume-image`).
- **Explicit non-goals (use Kind or GKE instead):**
  - Privileged host firewall / L7 Envoy `atenet-egress` containment and `MintActorJwt`.
  - Userspace RAM checkpointing (`runsc --checkpoint`); AX cold-start resume (`#683`) already persists state on `/workspace`.
  - Multi-node `WorkerPool` scheduling (`CreateWorker`, `DrainWorker`, etc.) and multi-tenant RBAC `AccessPolicy` RPCs (18 cluster-only RPCs tracked in `pkg/sublocal/drift_test.go`).

---

## Further Reading

- [`docs/quickstart-apple-container.md`](quickstart-apple-container.md) — Start on macOS Apple `container`
- [`docs/quickstart-podman.md`](quickstart-podman.md) — Start on rootless Podman (macOS & Linux)
- [`docs/architecture.md`](architecture.md) — Diagrams, `pkg/sublocal` embedding & configuration
- [`ghchinoy/ax-experiments/concepts`](https://github.com/ghchinoy/ax-experiments/tree/main/concepts) — Deep dive on AX vs. Agent Substrate 1.0 & datacenter-to-laptop lessons
- [`google/ax` Installation & Local Installer Docs](https://github.com/google/ax/blob/main/docs/local_installer.md) — Upstream Kind (`ax local`) and GKE setup
