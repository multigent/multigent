# Self-Hosted Sandbox Evaluation

Multigent will not depend on E2B Cloud. We need a self-hosted sandbox strategy that works in common environments, including WSL2 and cloud VMs where KVM may be unavailable.

## Decision

Default self-hosted runtime should be:

```text
Docker-compatible image model + gVisor runsc runtime
```

More precisely:

- Product default: sandbox-only execution, no host direct run.
- Self-hosted default: Docker Engine or containerd with `runsc`.
- Preferred gVisor platform: `systrap` for environments without KVM.
- Optional high-performance path: gVisor `kvm` only on bare-metal hosts with `/dev/kvm`.
- Fallback for developer bring-up only: Docker/runc with strict hardening.
- Not default: bubblewrap, nsjail, Firecracker, Kata, E2B self-host.

This preserves Docker's practical packaging model while replacing runc's normal shared-kernel container boundary with gVisor's application-kernel boundary.

This is not a promise that gVisor is perfect or fully compatible with every development workload. It is the best default under our current constraints: self-hosted, no E2B Cloud, no required KVM, agent code treated as untrusted, and Docker-compatible environment packaging.

## Why Not E2B Self-Hosted First

E2B-style self-hosting and Firecracker-like systems are attractive, but they usually assume KVM or cloud infrastructure that we cannot require.

Our near-term requirements:

- works on WSL2-like environments
- works on ordinary Linux VMs without nested virtualization
- supports existing container images or image-like templates
- supports filesystem materialization
- supports CPU/memory/network limits
- can be installed by early customers without operating a microVM platform

This points away from KVM-dependent runtimes as the default.

## Option Comparison

| Option | KVM needed | Packaging | Isolation | Operational complexity | Fit |
| --- | --- | --- | --- | --- | --- |
| Docker/runc | No | Excellent | Medium/weak for untrusted multi-tenant agents | Low | Good fallback, not enough as default |
| Rootless Docker | No | Excellent | Better host protection than rootful Docker | Medium | Useful hardening layer, not complete by itself |
| Docker/containerd + gVisor runsc systrap | No | Excellent | Stronger than runc; syscall interception/application kernel | Medium | Best default self-hosted path |
| gVisor runsc KVM | Yes | Excellent | Stronger and often faster on bare metal | Medium | Optional optimized path |
| Kubernetes Pod/runc | No | Excellent | Not enough by itself | High | Scheduler only, not sandbox |
| K8s + gVisor RuntimeClass | No | Excellent | Stronger | High | Good enterprise/self-hosted scale path |
| bubblewrap | No, but needs user namespaces | Poor environment packaging | Depends entirely on policy args | Medium/high | Good helper for local process confinement, not main runtime |
| nsjail | No, but needs namespaces/cgroups/seccomp support | Poor environment packaging | Good process jail if configured well | High | Useful for narrow command sandboxes, not full agent env |
| Kata/Firecracker | Yes | OCI-compatible in some setups | Strong VM boundary | High | Future high-security path, not default |

Short version:

- Do not make plain Docker/runc the security boundary for a multi-tenant agent SaaS.
- Do not make KVM-dependent microVM runtimes the default while WSL2 and ordinary VMs are target environments.
- Do use the Docker/container image model because dependency packaging is a first-class problem for coding agents.
- Use gVisor `runsc` as the first stronger isolation layer because it keeps OCI/Docker/Kubernetes compatibility and can run without KVM via `systrap`.

## gVisor Notes

gVisor provides an OCI runtime named `runsc` and integrates with Docker and Kubernetes. This is important because Multigent still needs image/template packaging for Codex, Claude Code, language runtimes, package managers, and project dependencies.

gVisor platform choice:

- `systrap`: default since mid-2023, does not require KVM, better choice inside many VMs.
- `kvm`: requires `/dev/kvm`; best on bare metal, not suitable as universal default.
- `ptrace`: older/high-overhead path, no longer the preferred default.

This means WSL2/non-KVM environments are not automatically excluded if we use gVisor with `systrap`.

Open questions to validate with a proof of concept:

- Whether `runsc` works reliably in our target WSL2 distribution and kernel settings.
- Which coding-agent workflows break under gVisor syscall/network/filesystem constraints.
- Performance impact for common loops: package install, test run, build, ripgrep, git diff, browserless QA.
- Whether Docker Engine + `runsc` or containerd + `runsc` should be our first production integration.

## Docker Notes

Plain Docker/runc gives us image packaging, cgroups, network namespaces, filesystem mounts, and resource limits, but it should not be our security promise for untrusted agents.

If Docker/runc is used as fallback:

- never privileged
- no Docker socket mount
- no host network
- no whole workspace writable mount
- non-root user in container
- read-only root filesystem when possible
- explicit tmpfs/write paths
- dropped capabilities
- seccomp profile
- AppArmor/SELinux where available
- CPU/memory/pids/output/time limits

Rootless Docker can reduce daemon/runtime risk, but it has setup and networking limitations. It is a useful hardening option, not the primary product abstraction.

Docker remains important as an implementation substrate:

- Image build and caching are mature.
- Developer/tool ecosystems already publish Docker images.
- Resource controls and filesystem mounts map naturally to our runtime template model.
- gVisor can be introduced as a runtime below Docker/containerd rather than replacing the whole packaging stack.

## bubblewrap / nsjail Notes

bubblewrap and nsjail are lightweight process isolation tools based on Linux primitives such as namespaces, cgroups/resource limits, and seccomp.

They are not a complete agent runtime environment:

- no image/template ecosystem
- dependency installation is our problem
- network policy is our problem
- filesystem materialization is our problem
- cgroup behavior varies by host
- user namespace availability varies by distro/WSL/kernel settings

They may still be useful later:

- inner sandbox inside a container
- single command/tool isolation
- MCP tool execution wrapper
- policy experiments for shell command confinement

But they should not be Multigent's primary runtime provider.

The reason is product-level, not just security-level: a Multigent agent runtime needs a reproducible full environment, not just a jailed process. Once we add Python/Node/Go packages, CLI credentials, MCP tools, repo materialization, network policy, output collection, and API tokens, bubblewrap/nsjail alone would force us to rebuild much of a container runtime and image system.

## Kubernetes Notes

Kubernetes is useful for scheduling, quotas, service accounts, network policy, and enterprise operations. A normal Pod is not enough for untrusted agents.

If using Kubernetes:

```text
Kubernetes Job/Pod + RuntimeClass(gvisor or kata) + network policy + resource quota
```

For no-KVM environments, prefer gVisor RuntimeClass. For stronger isolation on supported hosts, Kata/Firecracker-backed RuntimeClass can be an enterprise option.

Kubernetes should be treated as the fleet scheduler, not the sandbox itself. The sandbox boundary still comes from the selected runtime class and the pod security/network/resource policy around it.

## Recommended Architecture

User-facing concepts:

- Runtime Template
- Isolation Tier
- Resource Limits
- Network Policy
- Connection Grants
- MCP Grants
- Skill Grants

Internal provider choices:

```text
local/self-hosted default:
  docker-compatible provider using runsc/gVisor systrap

self-hosted fallback:
  hardened docker/runc

enterprise scale:
  k8s job + RuntimeClass(gvisor)

high-security enterprise:
  k8s job + RuntimeClass(kata/firecracker) where KVM exists

tool-level process jail:
  bubblewrap/nsjail as inner wrapper
```

## Runtime Template Model

Runtime templates should describe environment requirements without exposing the backing provider:

```yaml
id: codex-go-node
agent_cli:
  vendor: codex
  version: 0.18.0
base:
  family: debian
  image: ghcr.io/multigent/runtime-codex-go-node:2026-07
packages:
  apt:
    - git
    - ripgrep
    - python3
    - nodejs
    - golang
setup:
  - codex --version
  - go version
  - node --version
policy:
  network: allowlist
  filesystem: materialized-run-workspace
```

The base image and the agent CLI should be versioned separately. The default base image should contain stable OS/language dependencies, while the selected CLI is installed by a versioned toolchain installer during sandbox initialization. See `docs/runtime-toolchain-architecture.md`.

Provider mapping:

- Docker/runc: run image directly with strict options.
- gVisor: run same image with `--runtime=runsc`.
- Kubernetes: run same image as Job/Pod with `runtimeClassName: gvisor`.
- Future Kata: run same image with `runtimeClassName: kata`.

## Multigent Security Rules

Regardless of provider:

- No host direct execution.
- Agent gets a per-run workspace, not the full Multigent workspace.
- Agent gets a scoped API token, not DB credentials.
- Agent calls Multigent API for tasks/messages/context/artifacts.
- Agent gets only authorized context bundles.
- Skills are copied or read-only mounted by version.
- MCP credentials are injected through explicit connection grants.
- Workspace/project/user credentials are never globally mounted.
- Run output is collected as artifacts/diffs/memory candidates.

## Implementation Plan

1. Remove `none` / direct host run from production runtime choices.
2. Keep a hidden dev-only escape hatch if absolutely needed for internal debugging.
3. Add runtime provider `docker-runsc`.
4. Add runtime provider capability detection:
   - Docker available
   - runsc installed
   - runsc platform systrap/kvm
   - cgroup support
   - user namespace support
5. Change Docker provider to accept runtime variant:
   - `runc`
   - `runsc-systrap`
   - `runsc-kvm`
6. Add run materialization before execution.
7. Stop mounting the whole workspace root.
8. Add scoped agent API tokens.
9. Add MCP/connection grants.
10. Add K8s + gVisor provider after local runsc provider works.

## Recommendation

Continue using Docker's packaging and operational ecosystem, but do not continue relying on plain Docker/runc as the main sandbox.

The right next step is:

```text
Docker image model + gVisor runsc systrap as default self-hosted sandbox
```

This best matches our constraints:

- no cloud dependency
- no KVM requirement
- works with Docker/K8s ecosystems
- stronger isolation than runc
- practical path from current codebase
- leaves room for Kata/Firecracker where KVM exists

## References

- gVisor overview: https://gvisor.dev/docs/
- gVisor platform guide: https://gvisor.dev/docs/user_guide/platforms/
- gVisor Docker quick start: https://gvisor.dev/docs/user_guide/quick_start/docker/
- gVisor Kubernetes quick start: https://gvisor.dev/docs/user_guide/quick_start/kubernetes/
- Kubernetes RuntimeClass: https://kubernetes.io/docs/concepts/containers/runtime-class/
- Docker Engine security: https://docs.docker.com/engine/security/
- Docker rootless mode: https://docs.docker.com/engine/security/rootless/
- Docker user namespace remap: https://docs.docker.com/engine/security/userns-remap/
- Docker seccomp profile: https://docs.docker.com/engine/security/seccomp/
- bubblewrap: https://github.com/containers/bubblewrap
- nsjail: https://github.com/google/nsjail
