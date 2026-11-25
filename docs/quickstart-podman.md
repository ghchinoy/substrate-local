# Quickstart: AX + `substrate-local` on Rootless Podman (macOS & Linux)

Run unmodified upstream [AX](https://github.com/google/ax) (`ax-server` + `ax` CLI) directly on **rootless Podman** — with zero Docker, zero Kubernetes/Kind, and **without switching macOS `podman machine` to `--rootful`**.

---

## 1. Prerequisites

- **Go 1.27+** (or set `export GOTOOLCHAIN=auto`)
- **Podman** (rootless works out of the box):
  - **macOS:**
    ```bash
    brew install podman
    podman machine init        # default rootless libkrun VM is fine
    podman machine start
    podman info --format '{{.Host.Security.Rootless}}'   # true
    ```
  - **Linux:** Any working rootless `podman` installation (`podman info` succeeds).

> **Why rootless works here:** Kind-based `ax local start` requires rootful mode because Kind applies privileged node sysctls and runs Substrate's privileged `atelet` DaemonSet. `substrate-local` runs each actor as a plain rootless container publishing `-p 127.0.0.1::<port>` and routes traffic through its embedded `atenet-router`, requiring no rootful daemon or privileged DaemonSet.

---

## 2. Start the Local Stack on Rootless Podman

Clone `substrate-local` and start with `AX_LOCAL_RUNTIME=podman`:

```bash
git clone https://github.com/ghchinoy/substrate-local.git
cd substrate-local
export GOTOOLCHAIN=auto

AX_LOCAL_RUNTIME=podman make up
```

What this starts (zero Apple `container` required):
1. **PostgreSQL (`:5432`)** — `postgres:17-alpine` running in rootless `podman`.
2. **`substrate-local` (`:50051`)** — `runtime: podman`, publishing each actor on `127.0.0.1:<ephemeralPort>` and proxying via embedded `atenet-router`.
3. **Unmodified `ax-server` (`:8090`)** — built automatically from `github.com/google/ax@v0.4.0-rc.2` (or local `AX_REPO=~/projects/ax`).

Point your shell at the local stack:

```bash
source .substrate-local/env
ax version
```

---

## 3. Run, Inspect, Suspend & Resume a Task

```bash
ax apply -f examples/task-local.yaml
ax resume task local-task
ax describe task local-task          # State: Running, Ready: True

ax ssh local-task -- sh -c 'echo "hello from rootless podman" > /workspace/note.txt && uname -rm'
ax suspend task local-task           # archives /workspace and stops the container
ax resume task local-task            # restores /workspace into a fresh container
ax ssh local-task -- cat /workspace/note.txt
# hello from rootless podman

ax delete task local-task
AX_LOCAL_RUNTIME=podman make down
```

---

## 4. Bonus (macOS): Dual-Runtime Mobility (`Apple container` ↔ Rootless `Podman`)

If you have both Apple `container` and rootless `podman` on macOS, you can run two independent local AX clusters simultaneously and test both `/workspace` migration and upstream `ax promote`:

```bash
make multi-up                        # Cluster A (:8090/:50051 apple) + Cluster B (:8091/:50052 rootless podman)
source .substrate-local/env

# Run on Cluster A (Apple container)
ATENET_ROUTER_ADDR=$ATENET_ROUTER_ADDR_A ax --server $AX_SERVER_A apply -f examples/task-local.yaml
ATENET_ROUTER_ADDR=$ATENET_ROUTER_ADDR_A ax --server $AX_SERVER_A resume task local-task
ATENET_ROUTER_ADDR=$ATENET_ROUTER_ADDR_A ax --server $AX_SERVER_A ssh local-task -- sh -c 'echo "from apple" > /workspace/proof.txt'

# Migrate /workspace snapshot to Cluster B (rootless Podman)
./scripts/ax-stack.sh migrate local-task
ATENET_ROUTER_ADDR=$ATENET_ROUTER_ADDR_B ax --server $AX_SERVER_B ssh local-task -- cat /workspace/proof.txt

# Or test upstream ax promote --suspend-source
ax --server $AX_SERVER_A promote task local-task --to-server $AX_SERVER_B --suspend-source --replace

make multi-down
```

Run the automated dual-runtime regression check anytime:

```bash
TEST_PODMAN=1 make test-compat
```

---

## Where to Look Next

- **macOS Apple `container` Quickstart:** [`docs/quickstart-apple-container.md`](quickstart-apple-container.md)
- **Choosing between `substrate-local`, Kind (`ax local`), and GKE:** [`docs/choosing-a-runtime.md`](choosing-a-runtime.md)
- **Architecture, diagrams & `pkg/sublocal` Go embedding:** [`docs/architecture.md`](architecture.md)
- **AX & Agent Substrate 1.0 mental-model guides:** [`ghchinoy/ax-experiments/concepts`](https://github.com/ghchinoy/ax-experiments/tree/main/concepts)
- **Multi-agent swarms, Genkit A2A/MCP & HITL examples:** [`ghchinoy/ax-experiments`](https://github.com/ghchinoy/ax-experiments)
