# Matrix Production Readiness

## Goal

Matrix is not production-ready just because the feature set is rich.

To call Matrix production-ready, the repo must support:

- explicit schema state
- repeatable migration
- bounded workspace storage growth
- operational inspection
- documented operator workflow

This document is the current production-readiness baseline implemented in-repo.

## Implemented Baseline

### 1. Schema versioning

Matrix now records an explicit vault schema state:

- `system.schema.version`
- `system.schema.initialized_at`
- `system.schema.updated_at`

Current schema support is implemented in:

- [schema.go](/home/jose/hpdev/Libraries/matrix/internal/logic/schema/schema.go)

Operational commands:

```bash
matrix vault migrate
matrix vault doctor
matrix doctor
```

### 2. Workspace retention

Workspace growth is now bounded by a configurable retention policy:

- `retention.workspace.timeline_max`
- `retention.workspace.memory_max`
- `retention.workspace.snapshots_max`

The policy is implemented in:

- [retention.go](/home/jose/hpdev/Libraries/matrix/internal/logic/workspace/retention.go)

Operational commands:

```bash
matrix workspace retention
matrix workspace retention --set --timeline-max 200 --memory-max 500 --snapshots-max 100
matrix workspace prune billing-api
matrix workspace prune --all
```

### 3. Storage diagnostics

`matrix doctor` now includes:

- runtime
- logging
- storage

The storage section includes:

- schema status
- workspace retention policy
- per-workspace footprint
- pruning indicators

### 4. Decision trace

Matrix now explains orchestration behavior through:

- workspace state
- workspace timeline
- workspace decisions
- snapshots

Operator and AI surfaces:

```bash
matrix workspace decisions billing-api
curl http://127.0.0.1:9091/v1/workspace-decisions?workspace_id=billing-api
```

Chat:

- `/why`
- `/decisions [workspace]`

## Operator Workflow

Recommended local production workflow:

```bash
matrix vault migrate
matrix readiness
matrix doctor
matrix workspace retention
matrix workspace prune --all
matrix run
```

Recommended periodic maintenance:

```bash
matrix doctor
matrix logs doctor
matrix workspace prune --all
matrix vault backup
```

## What Is Still Not Fully Closed

Matrix is much closer to production, but these areas still require explicit hardening before a strict production claim:

- soak and load testing under sustained concurrency
- failure-injection and crash-recovery drills
- threat model and security review
- secret rotation and ingress hardening review
- retention policies driven by age/size in addition to count-based bounds
- migration stories beyond the first explicit schema version

## Current Verdict

After this hardening pass, Matrix should be considered:

- product-grade
- operationally much stronger
- closer to production

But still not “finished forever”.

The next production-focused work should center on:

- load and failure testing
- security review
- backup/restore drills
- explicit release criteria
