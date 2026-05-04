# Noema diagnostic: agent final output fragmented across `agent.message.final` and `run.context.attached`

Date: 2026-04-27

## Summary

During Noema experience-only OpenCode runs, Matrix sometimes exposed useful final answer text outside the canonical `agent.message.final` event.

Observed shape:

- `agent.message.final` may be empty, partial, or truncated.
- `run.context.attached` with `delivery_status=delivered` may contain the useful final answer or continuation.

Noema has added a defensive `observation_gap` path so this does not become a reusable task failure scar. Matrix should still expose final output through one coherent final-output contract.

## Evidence

Primary artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-non-interference-coding-pressure-cold-warm-v14/runs/phase91-experience-coding-pressure-cold-warm-phase91-go-checkout-summary-001-opencode-active-learned-seed-7/matrix-trace.json
```

Run:

```text
run_id=run-9a772fd2-7ce5-4e9b-ad88-a1bfbacdec25
logical_session_id=d77bdc48-6821-4a93-b02d-d4965fc62a20
remote_session_id=ses_23138727effeccg4V3K0iulsjr
agent=opencode
```

Relevant events:

```text
2026-04-27T11:52:39.436030322Z run.context.attached status=delivered
message starts: ` field is never captured from rollback events ... **Fixed.** Two bugs in summary.go ...

2026-04-27T11:52:46.159557211Z agent.message.final status=completed
message: The bug is in summary.go ... 2. The `Ref

2026-04-27T11:52:46.159557211Z run.completed status=completed
```

Related v15 artifact:

```text
/home/jose/hpdev/Libraries/noema/programs/evaluation-platform/artifacts/phase91-non-interference-coding-pressure-cold-warm-v15/runs/phase91-experience-coding-pressure-cold-warm-phase91-go-checkout-summary-001-opencode-active-learned-seed-7/outcome-critic-request.json
```

That request had to pass both `agent_final_text` and `delivered_context_text` as output evidence so the judge could recover the actual final answer.

## Expected Contract

Final task output should be recoverable from one stable Matrix field or explicitly typed ordered output channel.

Acceptable fixes:

- `agent.message.final` contains the complete final response.
- or Matrix exposes an ordered final-output fragment stream.
- or content currently emitted through delivered `run.context.attached` is typed separately as agent output evidence rather than context-delivery metadata.

## Why This Matters For Noema

Noema post-task judges consume Matrix traces. If final output is fragmented across transport metadata, a judge can correctly notice an observation-quality problem but wrongly classify it as task failure unless Noema adds special handling.

Noema now treats this as `observation_gap`, excludes it from seedable failure-scar learning, and does not count it as a task failed fact. Matrix should still make the canonical output unambiguous.

## Matrix maintainer response

Accepted and fixed.

Root cause: the Zed ACP client stored a single session observer per remote
session. A live `attach_context` prompt on the same remote session could replace
the main run observer, so later provider chunks were captured by the context
attachment path instead of the canonical run path.

Fix implemented:

- ACP session observers now fan out per session instead of replacing each other.
- Unregistering one observer no longer removes sibling observers for the same
  session.
- Regression coverage added with `TestClientFansOutConcurrentSessionObservers`.
- The run trace documentation now states that `agent.message.final` and
  `outcome.summary` are the canonical final-output surfaces; `run.context.attached`
  is delivery evidence, not final-answer storage.

Verification:

```text
go test ./pkg/zedacp -run TestClientFansOutConcurrentSessionObservers -count=1 -v
```

Result: passed.
