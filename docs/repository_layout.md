# Repository Layout

This repository now has a single product core plus supporting documentation and tooling.

## Product Source Of Truth

The live product is the repository root.

The live product documentation is:

- [`docs`](./)

These two locations are the current source of truth for Matrix architecture, runtime behavior, product strategy, and operational guidance.

## Support Material

These root-level items are auxiliary, not product core:

- [`.agent`](../.agent)
  Local skills and workflows used during development.
- [`.claude`](../.claude)
  Local editor and tool settings.

## Generated Artifacts

Generated binaries, logs, archives, and temporary directories must not be treated as repository source:

- root generated artifacts such as `mock-agent`, `matrix-bin.gz`, `configs.tar.gz`, `http.log`
- generated files inside the product root such as `matrix`, `matrix.exe`, `mock-agent`, `opencode`, `dist/`, `*.log`, `coverage.out`, `.tmp-*`

These are ignored or should be removed when found in the tree.

## Cleanup Policy

When deciding whether something belongs in the repo:

1. If it is needed to build, test, run, or document the current Matrix product, keep it.
2. If it is process material for humans only, keep it separate from product truth and do not let it drive implementation decisions.
3. If it is generated output, do not keep it in version control.
