---
description: Procedure to update and maintain the local Agent Client Protocol (ACP) knowledge base.
---

# Updating ACP Knowledge Base Workflow

This workflow defines the deterministic procedure to update the Agent Client Protocol (ACP) knowledge base located in `.agent/knowledge/acp/`. It ensures Matrix remains compatible with the latest ACP standard.

## Prerequisites

Before starting, ensure you have reviewed the existing knowledge base:
- Execute `view_file` on `.agent/knowledge/acp/STATE.md`

## Workflow Steps

1. **Check Official Documentation**
   - Use `read_url_content` to fetch the latest introduction and architecture documentation from:
     - `https://agentclientprotocol.com/get-started/introduction`
     - `https://agentclientprotocol.com/get-started/architecture`
   - Use `read_url_content` to fetch the latest reference specification from:
     - `https://agentclientprotocol.com/reference/overview`

2. **Check GitHub Releases and Changelog**
   - Use web search or `read_url_content` to query the official repository: `https://github.com/agentclientprotocol/agent-client-protocol`
   - Review recent commits, issues, or release notes to identify new capabilities, methods, or architectural shifts.

3. **Update Local Knowledge Base**
   - Synthesize the new information.
   - Use `write_to_file` or `replace_file_content` to update `.agent/knowledge/acp/STATE.md` with:
     - New transport layers (e.g., if SSE support is added).
     - New JSON-RPC capabilities (e.g., changes to standard edit, terminal, read).
     - Any breaking changes to the JSON-RPC interface.

4. **Update Implementations (If Applicable)**
   - If breaking changes are found during the update:
     - Trigger a planning phase (use `task_boundary` Mode: PLANNING).
     - Assess the impact on the `matrix` AgentRouter and ACPClient.
     - Propose needed updates in `internal/middleware/agent.go` and `internal/providers/agents/acp_client.go`.
