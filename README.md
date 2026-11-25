# `substrate-local`

A standalone, zero-Kubernetes local implementation of the **[Agent Substrate](https://github.com/agent-substrate/substrate) `v1.0.0` Control API (`ateapi/v1.ControlServer`)** and **`atenet-router`** (`ate-target-actor` HTTP/1.1 + HTTP/2 `h2c` reverse proxy), backed by macOS native **Apple `container`** (`Virtualization.framework`) or **rootless Podman**.

It plugs directly into unmodified upstream **[AX](https://github.com/google/ax)** (`ax-server` + `ax` CLI) via standard flags (`--substrate-endpoint`, `--substrate-plaintext`, `--router-url`) — **zero `google/ax` fork, zero Kind, zero Docker, and zero rootful VM required**.

---

## Why This Exists

Running full Agent Substrate locally via Kind (`ax local start`) gives full production parity (including `atenet-egress` TLS Secret injection), at the cost of a 6–8 GiB rootful Linux VM, kernel $\ge 6.8$, and ~3–5 minutes on a fresh cluster.

`substrate-local` is a complementary lightweight local tier built directly against `go.ate.dev/substrate@v1.0.0`:

- **~3-second stack startup** on macOS Apple `container` microVMs or default rootless `podman` (`libkrun`).
- **0-RAM suspend / resume & snapshot `Tag` fan-out:** Archives `/workspace` to disk on `SuspendActor` / `CreateTag` and hydrates it on `ResumeActor` or `CreateActor(source_tag)`.
- **Embedded `atenet-router` (`h2c` + HTTP/1.1):** Multiplexes gRPC Control API + `ate-target-actor` routing on one listener (`:50051`) with transparent wake-on-request, supporting `ax-server --router-url` task pushes and `ax ssh`.
- **Cross-runtime dual-cluster mobility:** Run Cluster A on Apple `container` and Cluster B on rootless `podman` simultaneously to test `ax promote` and `/workspace` migration locally.
- **Embeddable Go library (`pkg/sublocal`):** Zero dependency on `github.com/google/ax`; import `github.com/ghchinoy/substrate-local/pkg/sublocal` directly in Go tools or test harnesses.

---

## Quick Start

### Prerequisites
- Go `1.27+` (`GOTOOLCHAIN=auto`)
- macOS **Apple `container`** (`/usr/local/bin/container`, `container system start`) and/or **Podman** (`podman machine start` — rootless works out of the box)
- Upstream **[`google/ax`](https://github.com/google/ax)** checkout (defaults to `AX_REPO=~/projects/ax`) or `ax` / `ax-server` binaries

### 1. Run Standalone `substrate-local` Daemon
```bash
go install github.com/ghchinoy/substrate-local/cmd/substrate-local@latest

substrate-local --runtime apple --addr 127.0.0.1:50051 --snapshot-dir /tmp/substrate-snapshots
```

Point an unmodified upstream `ax-server` at it:
```bash
ax-server \
  --addr 127.0.0.1:8090 \
  --store "postgres://ax:ax@127.0.0.1:5432/ax?sslmode=disable" \
  --substrate-endpoint 127.0.0.1:50051 \
  --substrate-plaintext \
  --router-url http://127.0.0.1:50051
```

### 2. One-Command Local AX Stack (`make up`)
```bash
make up                                 # starts PostgreSQL, substrate-local, and upstream ax-server
source .substrate-local/env             # exports AX_SERVER=localhost:8090, ATENET_ROUTER_ADDR=127.0.0.1:50051, PATH

ax apply -f examples/task-local.yaml
ax resume task local-task
ax describe task local-task             # State: Running, Ready: True
ax ssh local-task -- uname -sm          # Linux aarch64
ax suspend task local-task              # State: Suspended (0 RAM, /workspace snapshotted)
ax resume task local-task
ax delete task local-task

make down
```

### 3. Dual-Cluster Cross-Runtime Mobility (`Apple container` $\leftrightarrow$ Rootless `Podman`)
```bash
make multi-up                           # Cluster A (:8090/:50051 apple) + Cluster B (:8091/:50052 podman)
source .substrate-local/env

ATENET_ROUTER_ADDR=$ATENET_ROUTER_ADDR_A ax --server $AX_SERVER_A apply -f examples/task-local.yaml
ATENET_ROUTER_ADDR=$ATENET_ROUTER_ADDR_A ax --server $AX_SERVER_A resume task local-task
ATENET_ROUTER_ADDR=$ATENET_ROUTER_ADDR_A ax --server $AX_SERVER_A ssh local-task -- sh -c 'echo "from apple" > /workspace/proof.txt'

./scripts/ax-stack.sh migrate local-task

ATENET_ROUTER_ADDR=$ATENET_ROUTER_ADDR_B ax --server $AX_SERVER_B ssh local-task -- cat /workspace/proof.txt
make multi-down
```

---

## Upstream AX `v0.4.0-rc.2` Compatibility & Regression Suite

Run the built-in contract regression suite against any unmodified upstream `google/ax` checkout (`AX_REPO`, default `~/projects/ax`):

```bash
make test                               # Go unit + live Apple container tests (~4s)
make test-compat                        # fresh upstream hack/tour.sh + PID-1/plain-image + h2c ax ssh stress (~45s)
TEST_PODMAN=1 make test-compat          # also tests Apple container <-> rootless Podman migrate & ax promote
```

Verified against **`google/ax` `v0.4.0-rc.2`+ (`8d879b5`)**:
- **Passes:** `Task` lifecycle (`Suspended`/`Running`), `Workspace` setup (`git` + `files` + `commands`) & `Tag` snapshot fan-out isolation, `--router-url` inbound task push, launcher contract (`AX_*_FILE` + `AX_METADATA_URL`), `ax ssh` (`h2c`), `suspend`/`resume` `/workspace` persistence, multi-tenant atespaces, `Model` CRUD, `ax promote`, and cross-runtime migration.
- **Known Limitations:**
  1. **Volumes-fidelity snapshots (`SNAPSHOT_FIDELITY_VOLUMES`):** Archives `/workspace` on suspend/resume; in-memory process state is not checkpointed (no `runsc --checkpoint`).
  2. **No `atenet-egress` L7/TLS proxy or Kubernetes Secret injection:** `ActorEgressPolicy` RPCs are stored so `Gateway` validation succeeds, but outbound internet access is not blocked on the host container bridge and Kubernetes Secret header injection requires a full Agent Substrate cluster (`ax local start`).

See [`docs/architecture.md`](docs/architecture.md) for architecture details, package usage (`pkg/sublocal`), and troubleshooting.
