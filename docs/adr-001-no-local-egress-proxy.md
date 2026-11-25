# ADR-001: Keep `atenet-egress` & Credential Injection Strictly on Kind / GKE

- **Status:** Accepted (2026-11-25)
- **Tracking:** `substrate-local-mtv`

## Context

In production [Agent Substrate](https://github.com/agent-substrate/substrate) (`v1.0.1`) and Kind-based `ax local start`, outbound network isolation and keyless API credential injection are enforced by **`atenet-egress`** (Envoy + `k8s-credential-provider`):

1. `Gateway` specs map to `EgressPolicy` (`HttpsRule.Effects.ReplaceHeaders`) carrying `ate-secret://k8s.io/default/<ns>/<secret>/<key>` references.
2. Inside the sandbox, `ax-runner` (`runner/runner.go` + `runner/env.go`) detects `/run/ate/egress-trust-bundle/trust-bundle.pem` and automatically sets `SSL_CERT_FILE`, `CURL_CA_BUNDLE`, `REQUESTS_CA_BUNDLE`, `NODE_EXTRA_CA_CERTS`, and `GIT_SSL_CAINFO`.
3. Outside the sandbox, `atenet-egress` intercepts outbound TCP:443 via pod network-namespace rules, verifies hostname allowlists and SSRF protections (blocking loopback/link-local/metadata IPs), terminates TLS with the trust-bundle CA, swaps placeholder headers (`x-goog-api-key`, `Authorization`) with real Kubernetes Secret values, and originates TLS upstream.

We evaluated whether `substrate-local` should add an opt-in userspace `HTTPS_PROXY` + TLS-MITM credential-injection proxy (backed by a local secrets file and a generated CA written to `/run/ate/egress-trust-bundle/trust-bundle.pem`) or leave `atenet-egress` strictly to Kind and GKE.

## Analysis

### What an opt-in userspace proxy would give us
- Unmodified `Gateway` manifests using `ate-secret://...` (such as `hack/hero-demo/gateway.yaml` and `hack/tour.sh` Step 8) could inject local API keys into cooperative HTTP/SDK clients without placing real keys in `Task.spec.env`, reusing `ax-runner`'s built-in `/run/ate/egress-trust-bundle/trust-bundle.pem` hook.

### Why it is the wrong trade-off for `substrate-local`
1. **Voluntary proxy ≠ security boundary:** On macOS Apple `container` (`vmnet`) and rootless Podman (`pasta`), `HTTPS_PROXY` is strictly voluntary unless host firewall rules are mutated (which violates zero-root). Any binary ignoring `HTTPS_PROXY` (`curl --noproxy '*'`, raw sockets, Go `http.Transport{Proxy: nil}`) bypasses the proxy and reaches the internet directly—creating a dangerous false sense of containment if users mistake a dev proxy for zero-egress enforcement.
2. **Disproportionate TLS-MITM surface area:**
   - Setting `SSL_CERT_FILE` / `REQUESTS_CA_BUNDLE` *replaces* the default root CA store in OpenSSL, Python, and Go, so `trust-bundle.pem` must bundle full system/Mozilla roots alongside the ephemeral CA or `tlsPassthrough` hosts (`github.com`, `storage.googleapis.com`) break.
   - Streaming LLM APIs (SSE / HTTP/2) require careful zero-buffer proxying and per-host leaf certificate caching.
   - Under rootless Podman (`libkrun`), outbound connections share a single NAT source IP, so attributing traffic to a specific actor's `EgressPolicy` would require either per-actor proxy listeners or embedding credentials in `HTTPS_PROXY` (which breaks several SDKs).
3. **Duplicates what Kind (`ax local start`) already does faithfully:** Upstream's Kind tier already runs real Envoy `atenet-egress` + Kubernetes Secrets with zero custom MITM code to maintain.

## Decision

**Do not implement a local `atenet-egress` or TLS-MITM credential-injection proxy in `substrate-local`.**

- `substrate-local` will continue to store and return `ActorEgressPolicy` RPCs on the Control API so `ax-server` and `Gateway` manifest validation pass cleanly, while documenting clearly that outbound egress enforcement and Kubernetes Secret injection require Kind (`ax local start`) or Google Cloud GKE ([`docs/choosing-a-runtime.md`](choosing-a-runtime.md)).
- For trusted local development on `substrate-local`, developers can either pass dev credentials via `Task.spec.env` or perform authenticated API calls outside the sandbox in an external orchestrator ([`docs/go-app-integration.md`](go-app-integration.md)).

## Revisit Trigger

Reopen this decision only if a high-value workflow requires `ate-secret://` header substitution on environments where Kind cannot run. If ever implemented, it must be strictly opt-in (e.g. `--dev-credential-proxy`) with an explicit startup warning that it provides **developer credential convenience only, not egress containment**.
