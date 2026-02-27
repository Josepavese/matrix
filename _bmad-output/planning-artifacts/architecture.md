---
stepsCompleted:
  - step-01-init
  - step-02-context
  - step-03-starter
  - step-02-context
inputDocuments:
  - _bmad-output/planning-artifacts/prd.md
  - docs/matrix_v2_vision.md
  - docs/matrix_v2_tech_spec.md
workflowType: 'architecture'
project_name: 'Matrix V2'
user_name: 'Jose'
date: '2026-02-26'
---

# Architecture Decision Document

_This document builds collaboratively through step-by-step discovery. Sections are appended as we work through each architectural decision together._

## Project Context Analysis

### Requirements Overview

**Functional Requirements:**
16 FRs spanning SSOT Management, Multi-Channel Routing, Package Management (APM), and Dual-Layer Interfaces:

> **IMPORTANT — Layer Priority:**
> 1. **Primary Interface (Core):** ACP (Agent Communication Protocol) and A2A (Agent2Agent Protocol) — structured REST/JSON-RPC protocols for agent-to-agent and agent-to-tool interaction. These are the main feature set of Matrix V2.
> 2. **Legacy/Compat Interface:** Virtual PTY Session Muxing and Semantic FUSE filesystem — retained for unstructured bash interaction and human-facing OS hooks only. These are NOT core features.

**Non-Functional Requirements:**
Critical constraints on Performance (<50MB RAM, <500ms startup, <50ms local latency) strictly limit architectural bloat. Strict Security (100% Local SSOT Vault encryption) eliminates stateful cloud backends. Furthermore, NFR-I1 guarantees strict compliance with ACP/MCP 1.0 specifications for all agent-exposed endpoints.

**Scale & Complexity:**
- Primary domain: System Daemon / Local CLI Tools / AI Orchestration
- Complexity level: High (Hybrid Protocol Management: TCP/JSON-RPC for agents + FUSE/PTY hooks for OS)
- Estimated architectural components: ~6 (Core Orchestrator, ACP/MCP Server, FUSE Mapper, PTY Muxer, APM Engine, DB Vault)

### Technical Constraints & Dependencies

- Must be compiled as a single binary in Go.
- **Hybrid Communication Protocol:** The system must gracefully handle concurrent, strictly-typed JSON-RPC streams (ACP) alongside raw byte stream multiplexing (Virtual PTY).
- Direct Linux FUSE integration requirements initially.

### Cross-Cutting Concerns Identified

- **Concurrency Lifecycle:** Safe multiplexing and termination logic for both long-running ACP TCP connections and legacy PTY processes without leaking memory.
- **Protocol Translation:** Maintaining consistent Agent State regardless of whether the ingress is a structured JSON-RPC call or an unstructured CLI text stream.
- **Security Boundary:** Ensuring FUSE commands cannot escape the `/mnt/matrix` jail and APM payloads are securely sandboxed.

## Starter Template Evaluation

### Primary Technology Domain

System Daemon / Command Line Interface (CLI) based on Go (Golang).

### Selected Starter: Custom Go Standard Layout + Cobra

**Rationale for Selection:**
For a high-complexity system daemon like Matrix V2, boilerplate generators introduce unnecessary dependencies that violate our NFRs (<50MB RAM limit). Using the Standard Go Project Layout (`/cmd`, `/internal`, `/pkg`) ensures modularity. `spf13/cobra` handles the complex nested CLI routing required by the APM (`matrix install`, `matrix config`).

**Initialization Command:**

```bash
mkdir matrix-v2 && cd matrix-v2
go mod init github.com/yourusername/matrix-v2
go get -u github.com/spf13/cobra@latest
# Install strict linter
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.56.2
```

### Architectural Foundational Rules (Provided by Starter Config)

**1. Extreme Code Quality & CI/CD Guardrails:**
A highly restrictive `.golangci.yml` configuration utilizing the `version: "2"` schema. 
- All default linters are disabled and only explicitly chosen linters are re-enabled.
- Enforced Linters: `staticcheck`, `revive`, `gocritic`, `gocyclo` (to restrict cyclomatic complexity in Muxing routines), `goconst`, `mnd` (no magic numbers), and `errcheck`. 
- Every PR will fail if even a single unhandled error or magic number is detected.

**2. Strict Platform Abstraction Layer (PAL):**
Any code touching the OS (FUSE), hardware, processes (PTY), or networking must flow through three strict layers:
- `Logic Layer`: Handles orchestration and intents. **Forbidden** to use `syscall` or check `runtime.GOOS`.
- `Middleware Layer`: Route providers and map normalized semantic errors.
- `Provider Layer`: Concrete implementation (e.g., Linux FUSE specific API calls).
Dependency direction is strictly `Logic -> Middleware -> Provider`.

**3. Single Source of Truth (SSOT) Supremacy:**
Maniacal consolidation of parameters, paths, and configurations. No hardcoded configuration values/constants are allowed in business logic. Everything must be sourced from a single configuration file structure (YAML) or the embedded database Vault, injected reliably into the application.

**Code Organization:**
- `/cmd/matrix`: The main entrypoint for the CLI/Daemon.
- `/internal/`: Private application and business logic (FUSE, PTY, APM).
- `/pkg/`: Public library code (e.g., ACP/MCP structs) that other agents might import.

## Core Architectural Decisions

### Decision Priority Analysis

**Critical Decisions (Block Implementation):**
- System Storage / Local Vault Strategy
- Inter-Process Communication (Daemon Muxing)

**Important Decisions (Shape Architecture):**
- Authentication & Identity validation for incoming RPC calls
- Process isolation enforcement (FUSE mounts)

### Data Architecture

**Decision: Local Vault & State Storage**
- **Choice:** `bbolt` (BoltDB)
- **Rationale:** Meets the rigid NFR constraints for memory footprint (<50MB RAM) and zero-CGO compilation. Essential for storing encrypted API keys (Vault) and tracking volatile state like active Virtual PTY session metadata without the overhead of an SQL engine.
- **Affects:** SSOT implementation, Session Management, API Key Vault.
- **Provided by Starter:** No (explicit architectural choice).

### API & Communication Patterns

**Decision: Primary Protocol Stack — ACP + A2A**
- **Choice:** ACP (Agent Communication Protocol, IBM/Linux Foundation) + A2A (Agent2Agent Protocol, Google/Linux Foundation). These two converging standards form the **primary interface layer** of Matrix V2.
  - **ACP:** RESTful HTTP interface for synchronous, asynchronous, and streaming agent runs. Local-first, low-latency. Defines `Runs`, `Messages`, and `Agent Manifest`.
  - **A2A:** JSON-RPC 2.0 over HTTP for peer-to-peer agent coordination. Defines `Agent Cards` (`/.well-known/agent.json`), `Tasks`, `Messages`, and `Artifacts`. Supports SSE streaming.
- **CLI-to-Daemon:** The existing JSON-RPC over TCP daemon serves as the internal management channel (Vault RPC). ACP/A2A are the external agent-facing protocols on the HTTP layer.
- **Affects:** Agent orchestration, multi-agent routing (A2A), CLI-to-Daemon internal management.
- **Provided by Starter:** No (explicit architectural choice).

**Legacy/Compat Protocol Stack — PTY + FUSE** *(lower priority)*
- **PTY:** Virtual pseudo-terminal muxing for unstructured shell interactions. Human-facing only.
- **FUSE:** Semantic filesystem exposing agent queries as native file I/O. Retro-compat for bash scripts.
- These are NOT core features and should not receive architectural priority over ACP/A2A.

### Authentication & Security

**Decision: Local Daemon Authentication**
- **Choice:** Ephemeral Local Tokens / OS-level Process Ownership
- **Rationale:** Since JSON-RPC over TCP exposes a local port, the daemon must verify that the incoming connection originates from the legitimate `matrix` CLI binary run by the identical localized user profile. OS-level peer credential checking (where applicable) paired with ephemeral token exchange ensures security.
- **Affects:** Daemon Security, System Interoperability.

### Decision Impact Analysis

**Implementation Sequence:**
1. ✅ Scaffold Go project with Cobra and `golangci-lint` config.
2. ✅ Build the Platform Abstraction Layer (PAL) interfaces.
3. ✅ Integrate `bbolt` as the core Provider for the SSOT Vault.
4. ✅ Implement the internal JSON-RPC over TCP daemon (`matrix run`).
5. ✅ APM (Agent Package Manager) — install/uninstall/update AI agent binaries.
6. ✅ PAL compliance audit and remediation.
7. **[NEXT]** `matrix config set/get` — SSOT configuration CLI.
8. **[NEXT]** ACP HTTP Server — `POST /runs`, `GET /runs/{id}`, Agent Manifest.
9. **[NEXT]** A2A Protocol — Agent Card, Task delegation, SSE streaming.
10. *[Future]* PTY Muxer — legacy compat session persistence.
11. *[Future]* FUSE Semantic Mount — legacy compat file I/O AI queries.

## Implementation Patterns & Consistency Rules

**Decision: Package Management (APM)**
- **Choice:** Meta-Orchestrator over Native Ecosystems (npm, pip, go, brew)
- **Rationale:** APM tracks installed AI agents by mapping logical identifiers (e.g., `gemini`, `codex`) to reliable, cross-platform global installer commands within their native ecosystems (e.g., `npm install -g`).
  - Strict Memory Footprint: The APM does not download blobs; it spawns sub-processes (`os/exec`) to trigger existing package managers.
  - State Detection: A package is categorized as "Installed" globally if `exec.LookPath(binaryName)` resolves its executable.
- **Affects:** Agent Registry, Installer CLI logic, Execution Pathing.

### Pattern Categories Defined

**Critical Conflict Points Identified:**
5 critical areas where AI agents could generate incompatible code if not strictly constrained: Naming Conventions, Error Handling, Project Organization, Inter-Process Communication (IPC), and Testing Methodology.

### Naming Patterns

**Go Code Naming Conventions:**
- **Files & Packages:** Strictly `snake_case` (e.g., `agent_router.go`). Package names must be singular, short, and lowercase without underscores (e.g., `package router`).
- **Structs & Interfaces:** `PascalCase` for exported entities (`UserSession`), `camelCase` for unexported (`userSession`). Interfaces typically end in `-er` (`VaultReader`) unless they represent core domain concepts (`Agent`).

**Database (bbolt) Keys:**
- Must use hierarchical `dot.notation` or `snake_case` (e.g., `api.keys.openai` or `active_session_123`).

### Format Patterns

**API & Data Exchange:**
- **JSON Payloads:** All fields must strictly use `camelCase` to ensure flawless interoperability with TypeScript/Frontend clients (e.g., `{"sessionId": "xyz", "activeModel": "gpt-4"}`).

### Process Patterns

**Error Handling (Strict PAL Compliance):**
- Providers (bbolt, OS, FUSE) will generate raw Go errors (e.g., `os.ErrNotExist`).
- **CRITICAL:** The Middleware layer MUST intercept and wrap these raw errors into custom normalized Go Errors containing: `code` (unique string identifier), `message` (human-readable explanation), and `op` (the logical operation name). 
- Raw errors must never leak into the Logic layer or the `cmd` package.

### Structure Patterns

**Project Organization (Standard Go Layout):**
- No hidden `init()` functions deeply buried in packages. All initialization occurs explicitly in `/cmd/matrix/main.go` to strictly control startup sequences (meeting NFR constraints).
- PAL layers map directly to internal directories:
  - `/internal/logic/`: Core business rules.
  - `/internal/middleware/`: Interfaces and abstraction routing.
  - `/internal/providers/bolt/`: bbolt implementation details.
  - `/internal/providers/fuse/`: FUSE implementation details.

### Communication Patterns

**JSON-RPC Naming:**
- RPC methods exposed by the daemon must be prefixed by their domain (e.g., `Agent.Call`, `Session.Attach`, `Vault.Unlock`).

**Logging:**
- All logs must be structured JSON utilizing the standard library `log/slog`.
- No third-party logging frameworks (like Logrus or Zap) are permitted to maintain the strict <50MB RAM footprint.
- Application logs go exclusively to `stderr`, reserving `stdout` for pure Agent-to-Agent text streams.

### Testing Methodology

**Hybrid Two-Tiered Testing Strategy:**
To align seamlessly with classic Go paradigms and BMAD workflow automation (`qa-generate-e2e-tests`), Matrix V2 employs two rigid tiers of testing:

1.  **Level 1: In-Place Unit Tests (Platform/Providers)**
    *   Test files (e.g., `bolt_test.go`) MUST reside in identical directories as the logic they evaluate.
    *   **Purpose:** Tests single provider logic or domain calculation, enabling zero-friction coverage calculation (`go test -cover`).
2.  **Level 2: Global E2E / Integration (`tests/` directory)**
    *   A root-level `tests/` directory dictates integration checks, E2E CLI bash simulations (via Go's `testscript`), and auto-generated mocks (via `gomock`).
    *   **Purpose:** Ensures components interact flawlessly across the PAL boundary without contaminating the `/internal` tree with mock files.

### Enforcement Guidelines

**All AI Agents MUST:**
- Adhere completely to the PAL rules: No `syscall` or `runtime.GOOS` logic outside of `/internal/providers`.
- Pass the `.golangci.yml` strictly defined linter suite with zero errors or magic numbers.
- Derive all configurable parameters logically from the SSOT (bbolt vault or centralized YAML).
