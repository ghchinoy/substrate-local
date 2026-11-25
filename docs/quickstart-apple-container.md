# Quickstart: AX + `substrate-local` on macOS Apple `container`

Get an unmodified upstream [AX](https://github.com/google/ax) control plane (`ax-server` + `ax` CLI) running on Apple Silicon macOS in ~30 seconds using native **Apple `container`** (`Virtualization.framework`) microVMs — no Docker, no Colima, and no Kubernetes cluster required.

---

## 1. Prerequisites

- **Apple Silicon Mac** running macOS 15+ with [Apple `container`](https://github.com/apple/container) installed at `/usr/local/bin/container`.
- **Go 1.27+** (or set `export GOTOOLCHAIN=auto` so Go downloads it automatically).

Verify Apple `container` is running:

```bash
container system start
container list
```

---

## 2. Start the Local Stack

Clone `substrate-local` and run `make up`. You do **not** need a local clone of `google/ax` — `ax-stack.sh` automatically fetches upstream `github.com/google/ax@v0.4.0-rc.2` via Go modules (or uses `~/projects/ax` if you already cloned it):

```bash
git clone https://github.com/ghchinoy/substrate-local.git
cd substrate-local
export GOTOOLCHAIN=auto

make up
```

`make up` starts three local components in order:
1. **PostgreSQL (`:5432`)** — `postgres:17-alpine` in an Apple `container` microVM.
2. **`substrate-local` (`:50051`)** — `ateapi/v1.ControlServer` + embedded `atenet-router` (`h2c`).
3. **Unmodified `ax-server` (`:8090`)** — connected via `--substrate-endpoint 127.0.0.1:50051 --substrate-plaintext --router-url http://127.0.0.1:50051`.

Point your shell's `ax` CLI at the local stack:

```bash
source .substrate-local/env
ax version
```

---

## 3. Run Your First Task & Shell Inside (`ax ssh`)

Apply the sample task using the public `google/ax` `v0.4.0-rc.2` `ax-runner` image:

```bash
ax apply -f examples/task-local.yaml
ax resume task local-task
ax describe task local-task
```

You should see `State: Running`, `WorkspaceReady=True`, and `Ready=True`. Shell into the live Linux `arm64` microVM via `substrate-local`'s embedded `atenet-router`:

```bash
ax ssh local-task -- sh -c 'echo "written inside Apple microVM" > /workspace/hello.txt && uname -sm'
```

---

## 4. Suspend to 0 RAM & Resume

Suspending archives `/workspace` to `.substrate-local/snapshots/` and stops the microVM (freeing 100% of its CPU and RAM). Resuming boots a fresh microVM and hydrates `/workspace`:

```bash
ax suspend task local-task
ax describe task local-task          # State: Suspended

ax resume task local-task
ax ssh local-task -- cat /workspace/hello.txt
# written inside Apple microVM

ax delete task local-task
```

---

## 5. Try a Warm `Workspace` + Snapshot Fan-Out

Declare a `Workspace` that clones a repo once into an immutable Substrate `Tag`, then fan out multiple tasks from that snapshot:

```bash
ax apply -f - <<'EOF'
apiVersion: ax.io/v1alpha1
kind: Workspace
metadata:
  name: quickstart-ws
spec:
  profile:
    image: "ghcr.io/google/ax/ax-runner@sha256:0937ebe5ad2d8db6bb25e88ded44d2d65de1b1bd8f0a9a97c6bb3787407d9947"
  setup:
    files:
      - path: README.txt
        content: "Prepared once, shared by every task.\n"
    commands:
      - "date -u > /workspace/built-at.txt"
EOF

ax describe workspace quickstart-ws   # Phase: Ready, SnapshotReady=True
```

Clean up when finished:

```bash
ax delete workspace quickstart-ws
make down
```

---

## Troubleshooting

- **Port `8080` warning on `make up`:** `ax-server` listens on `127.0.0.1:8090` to avoid colliding with common `:8080` dev tools. Always run `source .substrate-local/env` so `AX_SERVER=localhost:8090` and `ATENET_ROUTER_ADDR=127.0.0.1:50051` are exported.
- **`ax ssh` tries to run `kubectl port-forward`:** `ATENET_ROUTER_ADDR` is not set in your shell — run `source .substrate-local/env`.
- **Inspecting logs:** Run `./scripts/ax-stack.sh logs substrate-local` or `./scripts/ax-stack.sh logs ax-server`.
- **Starting completely clean:** Run `make reset` to remove leftover `ate-*` containers and restart fresh.

---

## Where to Look Next

- **Rootless Podman (macOS & Linux):** [`docs/quickstart-podman.md`](quickstart-podman.md)
- **Choosing between `substrate-local`, Kind (`ax local`), and GKE:** [`docs/choosing-a-runtime.md`](choosing-a-runtime.md)
- **Architecture, diagrams & `pkg/sublocal` Go embedding:** [`docs/architecture.md`](architecture.md)
- **AX & Agent Substrate 1.0 mental-model guides:** [`ghchinoy/ax-experiments/concepts`](https://github.com/ghchinoy/ax-experiments/tree/main/concepts)
- **Multi-agent swarms, Genkit A2A/MCP & HITL examples:** [`ghchinoy/ax-experiments`](https://github.com/ghchinoy/ax-experiments)
