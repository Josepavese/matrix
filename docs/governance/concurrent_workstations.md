# Working on Matrix from more than one session at a time

This repository has been worked on concurrently by more than one agent session on
the same workstation, sharing the same checkout. That is allowed, and it has
already caused two incidents worth writing down, because both were avoidable with
a rule rather than with judgement at the moment of the mistake.

## What went wrong, concretely

1. **A commit swept another session's work.** A `git add -A` staged an
   uncommitted, half-finished change from a different session that happened to be
   in the same working tree. It was pushed as part of an unrelated commit, the CI
   budget job failed on code the author of the commit had never seen, and the
   remote briefly claimed work that was not finished. The other session then had
   to commit on top of it to make the remote coherent again
   (`issues/closed/2026-09-22-artifact-digest-verification-consolidation.md`).
2. **A virtual machine was destroyed by someone who did not own it.** A QA guest
   named `qa-native-01` was created by another session to validate installers.
   Another session read the name as one of its own leftovers and ran
   `nido delete` on it, twice, destroying the bench while its owner was using it.

Neither incident involved malice or carelessness about the *work*; both came from
not being able to tell, at a glance, who or what something belonged to.

## Rules

**Commits**

- Stage explicit paths. `git add -A` and `git add .` are not used in this
  repository while more than one session shares the checkout.
- Read `git status` before committing and treat every path that is not yours as
  someone else's work in progress: leave it alone, do not stash it, do not commit
  it, do not revert it.
- If a commit of yours already contains someone else's change, say so in the
  commit message rather than quietly keeping it.

**Virtual machines and other shared resources**

- Names declare ownership: `matrix-*` belongs to the Matrix release workstream,
  `qa-*` belongs to installer/QA validation sessions, and anything else belongs to
  whoever created it.
- Never delete, stop or reconfigure a resource you did not create, even if it
  looks idle or abandoned. An idle guest is usually a paused validation, and a
  stopped one is usually waiting for a report. Ask through the issue channel
  instead.
- Before stopping anything, check what it is attached to: a `matrix run` process
  parented by `systemd --user` is an operator's installed runtime, not a leftover
  from a test.
- The Matrix vault is a bbolt database and takes a single writer. While the
  installed runtime is up it owns the vault; a CLI command that needs to open it
  read-write will fail with `ERR_VAULT_OPEN` timeout. That is expected, not a bug
  to work around by killing the runtime.

**Reporting what you find outside your own repository**

- Cross-repository findings go to `issues/` as an untracked note with the date in
  the filename, describing the symptom, the evidence and what you deliberately did
  not touch. The receiving workstream triages it and closes it with a resolution
  section in `issues/closed/`.
