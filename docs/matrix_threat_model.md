# Matrix Threat Model

## Goal

Matrix is a local-first agent communication runtime. Its main risks are not the same as a hosted SaaS control plane.

The threat model must focus on:

- local secret exposure
- overpowered tool execution
- ingress abuse
- vault corruption or loss
- unintended data retention

This document is the current baseline threat model for local and self-hosted deployments.

## Assets

Primary assets:

- vault secrets
- agent endpoint definitions
- workspace memory and snapshots
- timeline and decision trace
- channel credentials
- local filesystem access
- process execution capability

## Trust Boundaries

Matrix crosses these boundaries:

1. channel ingress
2. Matrix control core
3. agent protocol adapters
4. local tool execution
5. vault storage

The most dangerous boundary is between:

- agent/tool requests
- local execution and filesystem capabilities

## Main Threats

### 1. Vault compromise

Risk:

- a local attacker reads the vault file
- secrets are exposed

Mitigations already present:

- secure file permissions
- optional encryption with configured master key
- `matrix vault doctor`
- `matrix vault seal`

Remaining operator requirement:

- configure the vault master key in any serious deployment

### 2. Tool overreach

Risk:

- an external agent requests filesystem or terminal operations beyond intended scope

Mitigations already present:

- Matrix owns the routing layer
- tool calls pass through Matrix-controlled adapters
- workspace affinity reduces accidental context drift
- decision trace makes orchestration more audit-friendly

Remaining risk:

- permissive execution remains dangerous if operators expose sensitive local paths

### 3. Ingress misuse

Risk:

- an untrusted caller invokes Matrix over HTTP

Mitigations already present:

- `X-Matrix-Key` support on HTTP surfaces
- versioned ingress surfaces
- channel-neutral typed APIs

Remaining operator requirement:

- do not expose Matrix ingress publicly without network controls and auth

### 4. Retention sprawl

Risk:

- workspace timeline, memory, and snapshots grow without bound
- sensitive material accumulates locally

Mitigations already present:

- retention policy
- workspace prune commands
- storage diagnostics in `matrix doctor`

### 5. Restore-time corruption

Risk:

- restoring a vault while runtime is active
- clobbering valid local state

Mitigations already present:

- restore refusal while runtime ports are active
- pre-restore backup creation
- explicit backup and restore commands

## Operational Rules

Recommended production rules:

- always configure a vault master key
- always review `matrix readiness`
- keep HTTP ingress behind network controls
- periodically run `matrix workspace prune --all`
- take vault backups before upgrades and restores
- never treat external agents as fully trusted local code

## Current Status

This is a baseline threat model, not a formal security audit.

What still remains outside the repo:

- formal security review
- adversarial abuse testing
- host-hardening guidance per OS
- secret rotation procedures for all channel/provider credentials
