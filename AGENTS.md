# Repository guide

Use the existing project documentation before making changes:

- `README.md` is the user-facing install and feature guide.
- `docs/CODEBASE-ARCHITECTURE.md` is the high-level implementation map.
- `docs/IMPLEMENTATION.md` records implementation state and historical release notes.
- `docs/RELEASE-VALIDATION.md` records evidence and validation limits.
- `TASKS.md` tracks roadmap work and open acceptance gates.

Keep historical release evidence distinct from the current development state. The published v0.1.11 Windows installer used Inno Setup; the Windows notification-listener candidate is an unsigned MSIX intended for Microsoft Store submission. Its `userNotificationListener` capability, explicit user consent, Store identity/submission/policy review, and signed Store publication are relevant gates; installation and runtime validation are not yet established. This notification task is host-only and does not call for TomTom/USB deployment.

For documentation follow-ups, edit the most relevant existing guide and preserve unrelated working-tree changes. Do not treat design documents as evidence of implemented behavior.
