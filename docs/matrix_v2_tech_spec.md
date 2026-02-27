# Matrix V2 / Zion: Technical Specification

**Target Language**: Go (Golang)
**Target OS**: Linux (Primary), macOS, Windows
**Core Architecture Patterns**: Platform Abstraction Layer (PAL), Single Source of Truth (SSOT)

---

## 1. System Architecture Overview

Matrix V2 is designed as a persistent system daemon that provides a unified, cross-channel interface for local and remote AI agents. The architecture is strictly layered to ensure maximum portability and maintainability according to the **Platform Abstraction Layer (PAL)** principles.

### The Three Layers (PAL Enforcement)

1. **Logic Layer (Domain)**:
   - Houses the core business rules: session management, agent routing, swarm debate coordination, and parsing logic.
   - Entirely agnostic to the underlying operating system. NO `os`, `syscall`, or `exec` imports are allowed here.
   - Communicates exclusively through Go interfaces defined in the Middleware layer.

2. **Middleware Layer (Abstraction & Contracts)**:
   - Defines the interfaces (Contracts) that the Logic Layer consumes.
   - Responsible for Provider resolution and routing based on the runtime environment (Linux vs Windows).
   - Normalizes errors from the OS/Providers into semantic domain errors (e.g., `ErrAgentProcessDied`, `ErrConfigInvalid`).

3. **Provider Layer (Implementation)**:
   - Implements the Middleware interfaces using OS-specific APIs.
   - Scoped implementations exactly to one platform boundary (e.g., `linux_pty_provider.go`, `windows_conpty_provider.go`).

---

## 2. Component Specifications

### 2.1 Single Source of Truth (SSOT) Vault

**Goal**: Eradicate fragmented JSON/YAML configs. All API keys, model preferences, and agent limits live in ONE place.

- **Data Store**: Embedded `BoltDB` (or `SQLite`), encrypted at rest.
- **Access Pattern**: The SSOT Vault is hidden behind an interface `SSOTRepository`. The Logic Layer queries this interface. No other system component is allowed to read config files from disk directly.
- **Example Contract (Middleware)**:
  ```go
  type SSOTRepository interface {
      GetProviderKey(providerName string) (string, error)
      GetDefaultModel(agentType string) (ModelConfig, error)
      UpdateProviderKey(providerName, key string) error
  }
  ```
- **File Format**: If exported/imported, the format must be strict YAML.

### 2.2 Virtual TTY / PTY Muxing (The Memory Preserver)

**Goal**: Allow an agent (like Opencode) to run continuously in the background, preserving its exact environmental state, while users send inputs from isolated channels (Telegram, CLI).

- **Implementation (PAL)**:
  - **Logic**: Requests an interactive session via `sessionManager.CreateAgentSession(agentID)`.
  - **Middleware Contract**:
    ```go
    type PTYManager interface {
        Spawn(cmd string, env []string) (PTYSession, error)
    }
    type PTYSession interface {
        WriteContext(data []byte) error
        ReadStream() (<-chan []byte, error)
        Kill() error
    }
    ```
  - **Providers**:
    - *Linux/macOS Provider*: Uses UNIX pseudo-terminals (`pty` / `os/exec` with `Setsid`).
    - *Windows Provider*: Uses ConPTY API for exact terminal emulation.

### 2.3 Semantic FUSE Mount (Filesystem as an Interface)

**Goal**: Expose an OS-level mount point `/mnt/matrix` where reading a file invokes an LLM stream.

- **Implementation (PAL)**:
  - **Logic**: Handles the prompt generation based on the requested "filename" (e.g., `explain_db.md` -> prompt: "Explain the database structure").
  - **Middleware Contract**:
    ```go
    type VirtualFSMount interface {
        Mount(mountPoint string, handler FSHandler) error
        Unmount(mountPoint string) error
    }
    ```
  - **Providers**:
    - *Linux*: Uses `libfuse` via standard Go bindings (`bazil.org/fuse`).
    - *macOS*: Uses `macFUSE`.
    - *Windows*: Uses `WinFSP`.

### 2.4 Multichannel Event Router

**Goal**: Unify incoming messages from CLI, Telegram, WhatsApp, and REST.

- All channel listeners map incoming proprietary payloads into a standardized `MatrixMessage` struct.
- The Matrix Daemon receives `MatrixMessage`, checks the associated `SessionID`, retrieves context from the SSOT Vault and PTY Manager, and sends the prompt to the Agent Driver.

---

## 3. Directory Structure (Enforcing PAL & SSOT)

```text
 matrix-v2/
 ├── cmd/
 │   ├── matrixd/           # The System Daemon entrypoint
 │   └── matrix/            # The CLI tool entrypoint
 ├── internal/
 │   ├── logic/             # [LAYER 1] Business Rules. Pure Go. No OS imports.
 │   │   ├── session/       # Manages active agent sessions
 │   │   ├── swarm/         # Multi-agent debate orchestration
 │   │   └── apm/           # Agent Package Manager rules
 │   ├── mw/                # [LAYER 2] Middleware & Contracts
 │   │   ├── pty_contract.go
 │   │   ├── fs_contract.go
 │   │   └── ssot_contract.go
 │   └── providers/         # [LAYER 3] OS implementations
 │       ├── linux/
 │       │   ├── pty_linux.go
 │       │   └── fuse_linux.go
 │       ├── windows/
 │       │   ├── pty_conpty.go
 │       │   └── fuse_winfsp.go
 │       └── db/
 │           └── boltdb_ssot.go # Uses BoltDB for SSOT
 ├── go.mod
 └── README.md
```

---

## 4. Workflows & Sequences

### The APM (Agent Package Manager) Install Flow
1. User types `matrix install agent:opencode`.
2. **CLI** sends gRPC request to **matrixd**.
3. **Logic Layer** validates the request.
4. **Logic Layer** calls `Middleware.ContainerEngine.CreateEnvironment()`.
5. **Linux Provider** creates an isolated `chroot`/`venv`. **Windows Provider** creates an isolated user scope.
6. **Logic Layer** updates the **SSOT Vault** declaring the agent is installed and ready.

### The Swarm Interaction Flow
1. User requests code generation with validation via Telegram.
2. Telegram Gateway translates event to `MatrixMessage` and pushes to **Logic Layer**.
3. **Logic Layer** queries **SSOT Vault** for primary coder (`Opencode`) and primary reviewer (`Kimi`).
4. **PTY Provider** spawns `Opencode` PTY.
5. Coder output is captured via interface and piped back to **Logic Layer**.
6. **PTY Provider** spawns `Kimi` PTY for review.
7. Output loops until review tests pass.
8. Final payload returned via Gateway interface to Telegram.

---

## 5. Deployment & Configuration

- **Zero-Config Initiation**: On first run, `matrixd` initializes the SSOT database in `~/.matrix/vault.db`.
- **Global Config Management**: All external integrations (e.g., setting a Groq API key) are performed via CLI: `matrix config set provider.groq "<key>"`. This interacts strictly through standard gRPC to the daemon, which updates the SSOT database. No manual JSON editing.
- **Portability**: Compiled as a single static binary for Linux (`CGO_ENABLED=0` where FUSE is not required, or modular builds).
