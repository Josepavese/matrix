# Matrix Product Roadmap 2026-2027

## Goal

This roadmap translates the Matrix category thesis into a product plan.

The target is not "more features." The target is to make **Agent Communication Matrix** feel like a real category with a clear product shape, a visible wedge, and a credible expansion path.

The supporting thesis is documented in [matrix_category_thesis.md](matrix_category_thesis.md).

## Product North Star

Matrix becomes the **local-first communication crossroads for humans and external agents**.

The product should make one promise extremely clear:

> Every channel, every agent, and every handoff flows through one durable communication matrix.

## Product Layers

The product should be organized around four layers.

### 1. Communication Matrix Core

This is the non-negotiable core.

It includes:

- human-to-agent ingress
- agent-to-agent handoff semantics
- protocol-neutral conversation runtime
- session lifecycle control
- vault as SSOT and mirror
- discovery and activation
- channel-neutral ingress
- operator-facing diagnostics

### 2. Session Mobility And Continuity

This is where Matrix starts to feel different.

It includes:

- cross-channel continuity
- remote session import and reattach
- stable session identity across providers
- local mirror and recovery

### 3. Policy And Routing

This is where Matrix becomes an operator layer rather than just a runtime.

It includes:

- default routing policy
- agent capability profiles
- trust and cost routing
- workspace-aware routing
- escalation and handoff rules

### 4. Session Intelligence

This is the category-creating layer.

It includes:

- session timelines
- checkpoints and rollback
- summarization and compaction
- branch and fork semantics
- review and handoff flows between agents

## Current Baseline

Matrix already has a strong base in the first layer.

Already present in product or codebase:

- protocol-neutral core
- ACP and A2A adapters
- discovery-neutral onboarding
- typed session lifecycle actions
- channel-neutral runtime
- shared human-to-agent control semantics across channels
- local vault control
- local mirror of remote session metadata
- HTTP and Telegram ingress semantics built on shared session logic

This means the product does not need a reinvention. It needs focus and sequencing.

## What Matrix Should Sell First

The initial product wedge should be:

### Route real coding-agent communication from any channel through one matrix

That wedge is:

- specific
- demonstrable
- technically credible
- close to existing capabilities

It is better than selling:

- a generic orchestration runtime
- a multi-agent platform
- an "AI operating system"

## Roadmap

## Phase 0: Clarify The Product Surface

### Objective

Make the current product legible and category-consistent.

### Deliverables

- category thesis published
- product profile aligned to the thesis
- architecture docs aligned to the thesis
- clear split between supported and experimental surfaces

### Status

In progress and largely done in documentation.

## Phase 1: Make The Communication Matrix Obvious

### Objective

Turn the existing runtime into a product that obviously revolves around communication flows and session operations, not just message routing.

### Key Work

- strengthen `session` CLI and HTTP action surfaces
- expose identical lifecycle semantics on every channel
- make human-to-agent and agent-to-agent flows explicit in docs and APIs
- make remote mirror state more inspectable
- improve operator visibility of active, remote, mirrored, and detached sessions
- add explicit session provenance:
  - provider
  - protocol
  - workspace
  - channel affinity
  - last remote update

### Product Outcome

Users stop thinking "Matrix routes prompts" and start thinking "Matrix is where agent communication flows are controlled."

### Suggested Additions

- `matrix session inspect`
- `matrix session events`
- `matrix session where`
- richer `/session status`
- session labels beyond alias

## Phase 2: Workspace Affinity

### Objective

Bind sessions to real work context, not only to a channel identity.

### Why It Matters

Without workspace affinity, Matrix risks feeling like a better bot bridge.

With workspace affinity, Matrix becomes the persistent communication layer for real development work.

### Key Work

- bind sessions to workspace or repo identifiers
- persist workspace metadata in the vault
- route default agents by workspace policy
- show session-to-workspace mapping in operator surfaces
- support safe attach/resume from another channel or machine role

### Product Outcome

A session is no longer just "chat with gemini". It is "the session for repo X, branch Y, task Z".

## Phase 3: Policy Routing

### Objective

Make Matrix choose intelligently rather than simply remembering a default agent.

### Key Work

- routing profiles:
  - `cheap`
  - `fast`
  - `safe`
  - `offline-first`
  - `review`
- provider capability descriptors
- trust boundaries for tools and filesystem access
- escalation rules:
  - continue locally
  - escalate to stronger agent
  - hand off to reviewer

### Product Outcome

Matrix becomes the communication router that decides *where* work should flow, not only *how* to reach the current endpoint.

### Strategic Effect

This is where protocol neutrality turns into a practical advantage rather than a technical detail.

## Phase 4: Session Timelines

### Objective

Introduce the first unmistakably category-defining feature.

### Key Work

- append-only timeline per session
- append-only timeline per workspace
- checkpoints
- rollback
- summaries and compaction
- branch and fork semantics
- timeline import from remote provider metadata where possible

### Product Outcome

This is the first feature that genuinely moves Matrix out of "yet another runtime" territory.

### Why It Matters

Most agent systems treat history as chat. Matrix should treat history as an operational timeline.

The implementation spec for the first productized version is documented in
[matrix_workspace_timeline_spec.md](matrix_workspace_timeline_spec.md).

## Phase 5: Agent Handoff And Review Flows

### Objective

Let one session move across multiple agents while preserving continuity.

### Key Work

- explicit handoff between agents
- reviewer mode
- escalation path from cheap to strong
- comparison mode between providers
- multi-agent review trails attached to one session identity

### Product Outcome

The stable object becomes the session and task, not the individual provider.

## Phase 6: Operator Console

### Objective

Make Matrix feel like an operational console for agent systems.

### Key Work

- text-first console UX in CLI
- strong runtime status views
- per-agent health and readiness
- session fleet view
- recent failures and detached sessions
- policy audit and routing reasons

### Product Outcome

This helps Matrix avoid collapsing into "chat app with plugins."

## Capability Priorities

If sequencing must be strict, the order should be:

1. session inspectability
2. workspace affinity
3. policy routing
4. session timelines
5. agent handoff
6. richer operator console

This order preserves the category thesis and keeps the wedge grounded.

## Features To Avoid Leading With

These may still exist, but should not define the category.

- marketplace language
- generic "swarm" claims without strong operational semantics
- FUSE as the main story
- vague autonomous-agent branding
- too much emphasis on protocol count

Protocols matter, but they are not the category.

## Commercial Packaging

The likely packaging path is:

### Edition 1: Personal Operator

Audience:

- solo developers
- power users
- local-first AI users

Promise:

- one place where your real agents and channels meet

### Edition 2: Team Communication Matrix

Audience:

- small engineering teams
- AI-heavy dev teams

Promise:

- shared policy, visibility, and portable communication/session control

### Edition 3: Enterprise Edge Control Layer

Audience:

- orgs with heterogeneous agent stacks
- regulated environments

Promise:

- local control, auditability, and protocol-neutral communication governance

## Website And Messaging

Recommended homepage message:

### Headline

**Every channel. Every agent. One communication matrix.**

### Subheadline

Matrix is the local-first communication crossroads that routes human-to-agent and agent-to-agent work across protocols, channels, and workspaces without losing continuity, policy, or operational visibility.

### Proof Points

- Bring your own agents
- ACP today, A2A ready
- One communication surface across CLI, HTTP, and chat
- Local vault as control and mirror
- Switch, inspect, cancel, and recover without protocol knowledge

## Demo Strategy

The strongest demo is not a chatbot demo.

It should show:

1. start with one coding agent in terminal
2. continue from Telegram or HTTP
3. inspect and switch the session
4. hand off or escalate to another agent
5. recover or cancel from a different surface

That demo communicates the category much better than a generic "ask AI a question" flow.

## Strategic Decision

Matrix should optimize for:

- communication continuity
- heterogeneous agent control
- local-first governance
- session mobility

It should not optimize first for:

- prompt authoring
- agent building DSLs
- generic multi-agent spectacle

## Recommendation

The recommended roadmap commitment is:

### Short term

- make the communication matrix visible and inspectable
- add workspace affinity
- add policy routing

### Mid term

- ship session timelines
- ship handoff and review flows

### Long term

- package Matrix as the communication console for heterogeneous agent fleets

If executed well, this creates a category that is meaningfully separate from both agent builders and protocol projects.
