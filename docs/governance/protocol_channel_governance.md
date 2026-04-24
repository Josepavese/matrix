# Protocol And Channel Governance

Protocols and channels are separate axes.

Protocol rules:

- ACP support follows the Zed Agent Client Protocol model.
- A2A remains a first-class strategic target, but unsupported market behavior must be documented as capability-pending rather than simulated.
- Provider discovery, session lifecycle, fork, delete, cancel, and streaming behavior must flow through neutral interfaces.
- Raw protocol events may be preserved as source evidence, but Matrix must emit normalized events for channels and orchestration.

Channel rules:

- Telegram is one channel, not the product spine.
- HTTP is one channel, not a privileged internal shortcut.
- CLI is one channel and operator surface.
- Future channels must implement the same command vocabulary where technically possible.

Command parity is mandatory for workspace, session, provider, handoff, fork, delete, timeline, snapshot, and status flows. If a channel cannot support a command, it must return a clear capability error.
