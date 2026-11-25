# Integrating External Go Apps with Stock AX + `substrate-local`

This guide is for Go control planes and developer tools — such as repository auditors, CI/code-review services, or multi-agent orchestrators — that drive **unmodified upstream [AX](https://github.com/google/ax)** (`ax` CLI or `github.com/google/ax/client`) backed by `substrate-local`.

## Why Integrate This Way?

Rather than invoking container CLIs directly inside your application, driving stock `ax` against `substrate-local` gives an external Go app three durable advantages:

1. **Clean runtime decoupling:** Your application speaks a declarative, Kubernetes-shaped API (`Task`, `Workspace`, `Gateway`, `Model`) while `ax-server` and `substrate-local` handle sandbox provisioning, `--entrypoint` injection, `/workspace` snapshotting, and `h2c` guest exec.
2. **Fast, Docker-free local execution:** Developers and laptop-resident tools get ~3-second microVM/rootless-container isolation via Apple `container` or rootless Podman with 0-RAM suspend/resume — without requiring Docker Desktop or a 6–8 GiB Kind VM.
3. **Zero-rewrite path to production:** The exact same Go driver and Task manifests graduate unchanged to Kind (`ax local start`) or Google Cloud GKE simply by switching `AX_SERVER` or the kubeconfig context whenever a workload needs strict `atenet-egress` containment or multi-node autoscaling.

> **Key AX v0.4 architectural invariant (`google/ax#756`):** AX is a pure control plane. `TaskStatus` mirrors Substrate's `ActorState` (`Suspended`, `Running`, `Crashed`, …) and does **not** report command exit codes or outputs. One-shot tasks remain `Running` after their command finishes — the **external Go orchestrator** owns completion detection, artifact retrieval, and teardown.

---

## 1. Ephemeral One-Shot Audit Job Pattern

### Recommended Task Manifest (`examples/task-audit-job.yaml`)

For ephemeral jobs (like auditing a repository and returning a structured `verdict.json` + `patch.diff`):

1. **Use a unique task name per run** (e.g. `audit-<repo>-<shortid>`) in a dedicated `atespace` (e.g. `audits`), because AX `Task` resources are immutable. **Atespaces are auto-created on first `ax apply`** — both `ax-server` and `substrate-local` create `atespace: audits` automatically with no prior setup call.
2. **Pass structured inputs via `spec.config`** (mounted by `ax-runner` as JSON at `$AX_CONFIG_FILE`) and **`spec.goal.text`** (`$AX_GOAL_FILE`), or bind a pre-warmed `Workspace` when auditing the same repository across multiple checks.
3. **Set `spec.debug: true`** so the host Go app can stream inputs in and poll/fetch artifacts over `ax ssh`.
4. **Write artifacts first, then atomically publish `/workspace/.ax/exit-code` last** (`mv exit-code.tmp exit-code`) so polling never observes half-written JSON or diffs.

See the complete runnable manifest in [`examples/task-audit-job.yaml`](../examples/task-audit-job.yaml).

### Uploading Private Repositories Without Secrets in the Sandbox

When auditing a private repository, clone it on the host (keeping Git/SSH credentials strictly outside the sandbox), strip `.git/config` remote tokens, and stream the repository directly into `/workspace/repo` over `ax ssh` stdin:

```bash
ax -a audits ssh <task> -- mkdir -p /workspace/repo
COPYFILE_DISABLE=1 tar --no-xattrs -C /host/path/to/repo -czf - . \
  | ax -a audits ssh <task> -- tar -C /workspace/repo -xzf -
```

- **Verified throughput & integrity:** Streaming multi-megabyte tarballs (verified with 10 MiB binary payloads, exact SHA-256 match) works reliably over `substrate-local`'s `h2c` router. On macOS, setting `COPYFILE_DISABLE=1` and `--no-xattrs` avoids harmless BSD `LIBARCHIVE.xattr` warnings inside Linux GNU `tar`.
- **Coordinating start after upload (two options):**
  - **Marker handshake (best for long-running / detached audits):** Have `spec.commands` wait on a host-written ready marker before running (`while [ ! -f /workspace/.ax/ready ]; do sleep 0.2; done; <run-audit>`). After the `tar` upload finishes, the host runs `ax ssh <task> -- touch /workspace/.ax/ready` and then polls for `/workspace/.ax/exit-code`.
  - **Direct `ax ssh` exec (simplest for short synchronous audits):** Start the task with `commands: ["sleep", "3600"]`, stream the repo tarball via `ax ssh`, and execute the audit command directly via `ax ssh <task> -- <audit-cmd>` to capture stdout/stderr and the exit code synchronously without polling.

### Base Image Toolset vs. Language Images (`AX_RUNNER_VOLUME_IMAGE`)

- **Stock `ghcr.io/google/ax/ax-runner` (`v0.4.0-rc.2`, `python:3.12-slim` base) includes:** `python3` (`3.12`), `git`, `curl`, `ssh`, `bash`, `tar`, `ps` (`procps`), and `ca-certificates`. It does **not** include `go`, `node`, `npm`, `rustc`, or language-specific linters.
- **For Go, Node, Rust, or custom toolchains:** Start `ax-server` with `AX_RUNNER_VOLUME_IMAGE=ghcr.io/google/ax/ax-runner-volume@sha256:a85dc4ac3e4b060d9157e3f80c01463544e79a0e92106bacad1e4f3f3cf5e4d5` (`--runner-volume-image`), and set `spec.profile.image` to any digest-pinned language image (e.g. `golang:1.25-alpine@sha256:...` or `node:22-slim@sha256:...`). `substrate-local` pulls and caches the runner volume once and mounts it read-only at `/opt/ax` (`PID 1 = /opt/ax/ax-runner`) — no custom runner Dockerfile required.

### Go Orchestrator Lifecycle Pattern

```go
package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type StackConfig struct {
	AXBin            string // e.g. "ax" or "~/.substrate-local/bin/ax"
	AXServer         string // e.g. "localhost:8090" (avoids :8080 app ports)
	AtenetRouterAddr string // e.g. "127.0.0.1:50051"
	Atespace         string // e.g. "audits"
}

type AuditResult struct {
	ExitCode int
	Verdict  json.RawMessage
	Patch    string
}

func (c StackConfig) axCmd(ctx context.Context, args ...string) *exec.Cmd {
	fullArgs := append([]string{"--server", c.AXServer, "-a", c.Atespace}, args...)
	cmd := exec.CommandContext(ctx, c.AXBin, fullArgs...)
	cmd.Env = append(os.Environ(),
		"AX_SERVER="+c.AXServer,
		"ATENET_ROUTER_ADDR="+c.AtenetRouterAddr,
	)
	return cmd
}

func RunEphemeralAudit(ctx context.Context, cfg StackConfig, taskName string, manifestYAML []byte) (*AuditResult, error) {
	// Always clean up the sandbox even on error or timeout.
	defer func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = cfg.axCmd(cleanCtx, "delete", "task", taskName).Run()
	}()

	// 1. Apply immutable Task manifest.
	applyCmd := cfg.axCmd(ctx, "apply", "-f", "-")
	applyCmd.Stdin = bytes.NewReader(manifestYAML)
	if out, err := applyCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ax apply: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// 2. Resume starts the microVM/container and pushes the task over --router-url.
	if out, err := cfg.axCmd(ctx, "resume", "task", taskName).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ax resume: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	// 3. Poll for atomic completion marker over ax ssh (h2c via substrate-local).
	var exitCodeStr string
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		out, err := cfg.axCmd(ctx, "ssh", taskName, "--", "cat", "/workspace/.ax/exit-code").Output()
		if err == nil {
			exitCodeStr = strings.TrimSpace(string(out))
			break
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for %s completion: %w", taskName, ctx.Err())
		case <-ticker.C:
		}
	}

	// 4. Retrieve structured JSON verdict and optional patch.diff.
	verdictBytes, err := cfg.axCmd(ctx, "ssh", taskName, "--", "cat", "/workspace/.ax/verdict.json").Output()
	if err != nil {
		return nil, fmt.Errorf("reading verdict.json: %w", err)
	}
	patchBytes, _ := cfg.axCmd(ctx, "ssh", taskName, "--", "cat", "/workspace/.ax/patch.diff").Output()

	var code int
	_, _ = fmt.Sscanf(exitCodeStr, "%d", &code)
	return &AuditResult{
		ExitCode: code,
		Verdict:  verdictBytes,
		Patch:    string(patchBytes),
	}, nil
}
```

#### Optional: Using `github.com/google/ax/client` (gRPC) + `ax ssh`

If your Go app already imports `github.com/google/ax/client` (alpha), you can use typed gRPC RPCs for `CreateTask`, `ResumeTask`, `GetTask`, and `DeleteTask` while keeping `ax ssh` (or `internal/guest`) for file retrieval:

```go
axClient, conn, err := client.Dial(ctx, client.Options{
	ServerURL: "http://" + cfg.AXServer, // e.g. "http://127.0.0.1:8090"
	FromEnv:   true,                     // honors $AX_SERVER when ServerURL is empty
})
if err != nil {
	return err
}
defer conn.Close()
```

> **Tip for large multi-file outputs:** Instead of multiple `ax ssh -- cat ...` calls, stream an entire directory out in a single `h2c` call with `ax ssh <task> -- tar -C /workspace/.ax -czf - .` and unpack in Go using `archive/tar` + `compress/gzip`.

---

## 2. Discovering & Connecting to `substrate-local` + `ax-server`

Choose whichever of the three integration modes fits your app's deployment model:

### Mode A: Connect to a Running Local Stack via Env Vars *(Recommended Default)*

Keep `substrate-local` + `ax-server` running via `make up` (so PostgreSQL and cached OCI images stay warm across runs) and configure your Go app with two environment variables:

| Variable | Default (`ax-stack.sh`) | Used By |
|---|---|---|
| `AX_SERVER` | `localhost:8090` | `ax` CLI (`--server`) & `github.com/google/ax/client` |
| `ATENET_ROUTER_ADDR` | `127.0.0.1:50051` | `ax ssh` (routes `h2c` gRPC directly to `substrate-local`, skipping `kubectl port-forward`) |
| `AX_BIN` | `.substrate-local/bin/ax` or `ax` on `PATH` | External Go app invoking `ax` |

**Avoiding port `8080` collisions:** `scripts/ax-stack.sh` intentionally runs `ax-server` on **`:8090`** (not `:8080`) so external web UIs or API servers can bind `:8080` without conflict. Always pass `--server "$AX_SERVER"` explicitly rather than relying on `ax`'s `:8080` fallback.

**Auto-discovery fallback in Go:** Look for explicit `AX_SERVER` / `ATENET_ROUTER_ADDR` in `os.Getenv` first; if unset, optionally parse key=value lines from `<substrate-local-repo>/.substrate-local/env` (`export KEY=VALUE`), and run a quick preflight check before accepting jobs:

1. `GET http://<ATENET_ROUTER_ADDR>/healthz` → expects `200 OK` (`substrate-local` healthy).
2. `ax --server <AX_SERVER> get tasks` (or `axClient.ListTasks`) → verifies `ax-server` + PostgreSQL are reachable.

### Mode B: Spawn `substrate-local` as a Managed Subprocess

If your Go app bundles or discovers the `substrate-local` binary (installed from [GitHub Releases](https://github.com/ghchinoy/substrate-local/releases) or `go install`), launch it directly with isolated ports and snapshot directory:

```bash
substrate-local \
  --runtime apple \
  --addr 127.0.0.1:50051 \
  --snapshot-dir /path/to/app-data/snapshots
```

Poll `GET http://127.0.0.1:50051/healthz` until `200 OK`, then start `ax-server` with `--substrate-endpoint 127.0.0.1:50051 --substrate-plaintext --router-url http://127.0.0.1:50051`. Send `SIGTERM` on shutdown so `substrate-local` cleanly stops any running actor containers.

### Mode C: Embed `pkg/sublocal` Directly In-Process

Because `pkg/sublocal` has **zero dependency on `github.com/google/ax`**, Go apps and integration tests can embed the Substrate Control + Router daemon in-process on an ephemeral port (`127.0.0.1:0`):

```go
cfg := sublocal.DefaultConfig()
cfg.Runtime = sublocal.RuntimeApple // or sublocal.RuntimePodman
cfg.SnapshotDir = filepath.Join(dataDir, "snapshots")

srv, err := sublocal.NewServer(cfg)
if err != nil {
	return err
}
defer srv.Close(context.Background())

lis, _ := net.Listen("tcp", "127.0.0.1:0")
grpcServer := grpc.NewServer()
srv.Register(grpcServer)
httpServer := srv.NewHTTPServer(lis.Addr().String(), grpcServer)
go func() { _ = httpServer.Serve(lis) }()
```

---

## 3. Network-Isolated & Offline Audit Runs (`atenet-egress` Caveats)

Because `substrate-local` does **not** run an L7/TLS `atenet-egress` firewall proxy (see [`docs/choosing-a-runtime.md`](choosing-a-runtime.md)), containers launched by `substrate-local` inherit outbound internet access from the host bridge (`vmnet` on Apple `container`, `pasta` on rootless Podman).

Note also why `--network=none` is **not** a drop-in fix for `substrate-local`: `ax-server` pushes tasks over port `80` and `ax ssh` connects to `ax-runner` on port `80` via the container's network interface. Disabling container networking completely would cut off `ax-runner` as well.

Follow these four practices depending on your trust level:

1. **Zero secrets inside the sandbox (always):**
   - Never pass GitHub tokens, SSH keys, or LLM API keys in `Task.spec.env` when auditing third-party code.
   - Run deterministic/static analysis inside the sandbox, pull `verdict.json` and `patch.diff` out over `ax ssh`, and perform authenticated GitHub PR comments or LLM calls **outside the sandbox** in your host Go control plane.
2. **Pre-stage repositories and dependencies for offline repeatability:**
   - Clone/checkout the target repository on the host (or during a trusted `Workspace` setup step), strip `.git/config` remote credentials, and bake language toolchains/caches into the audit image so the audit runs with offline flags (`GOPROXY=off`, `npm ci --offline`, `pip --no-index`).
3. **Use `--runner-volume-image` for clean tool images:**
   - Start `ax-server` with `AX_RUNNER_VOLUME_IMAGE=ghcr.io/google/ax/ax-runner-volume@sha256:...` so your audit image only needs your audit tools (`git`, `python3`, linters) without baking `ax-runner` in.
4. **Graduate untrusted / hostile PR execution to Kind (`ax local`) or GKE:**
   - If your audit executes untrusted build scripts (`go test`, `npm test`, `Makefile`, `setup.py`) from arbitrary external PRs and requires **strict zero-egress containment** (blocking data exfiltration and arbitrary outbound connections while still allowing port-80 `ax ssh`), point the **exact same `StackConfig` / Task YAML** at a Kind (`ax local start`) or GKE Agent Substrate cluster and omit `spec.gateway`. Your Go code does not change — only `AX_SERVER` / kubeconfig context switches.
