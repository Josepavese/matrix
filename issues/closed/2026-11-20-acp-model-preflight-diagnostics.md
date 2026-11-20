# ACP model-selection refusal

**Decision.** Accepted for the Matrix diagnostic contract.
[GitHub #23](https://github.com/Josepavese/matrix/issues/23).

Matrix classifies a peer reply of `model not found` as
`provider_model_unavailable`, keeps the requested model and the bounded RPC
evidence, and reports the negotiated version and the actual wire method.
Authentication refusals retain their own code.

Cold-start probes using the official Linux OpenCode 1.18.32 and 2.2.5 binaries
returned ACP v1 and accepted the same model with a complete synthetic
provider profile. Removing the profile reproduced the peer refusal on both
versions. Once Matrix's agent readiness probe completed, an authenticated
Matrix HTTP run on each version returned HTTP 201 and completed against a
local synthetic inference endpoint. With the key file still present but the
profile absent, both returned HTTP 424 with `provider_model_unavailable` and
`session/set_config_option` before any inference request.

The reporting worker was running OpenCode 2.2.5, while the Hub contract
qualifies 1.18.32. The pinned registry and official 1.18.32 archive agree on
the checksum. The team that owns the worker must still qualify the effective
artifact, provider profile and cold-start projection, then repeat the
original turn. That verification is tracked in
[Half Pocket #36](https://github.com/Josepavese/gestionale-ai-halfpocket/issues/36).
