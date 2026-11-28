# Repository guide

Use the existing project documentation before making changes:

- `README.md` is the user-facing install and feature guide.
- `docs/CODEBASE-ARCHITECTURE.md` is the high-level implementation map.
- `docs/IMPLEMENTATION.md` records implementation state and historical release notes.
- `docs/RELEASE-VALIDATION.md` records evidence and validation limits.
- `TASKS.md` tracks roadmap work and open acceptance gates.

Keep historical release evidence distinct from the current development state. The published v0.1.11 Windows installer used Inno Setup; Windows continues to use Inno Setup and the in-app updater.

For documentation follow-ups, edit the most relevant existing guide and preserve unrelated working-tree changes. Do not treat design documents as evidence of implemented behavior.

## Releases and versions (mandatory for every agent)
Before bumping a version, tagging, packaging or publishing, follow `docs/RELEASE-PROTOCOL.md` (also available as the `release-prep` skill in `.claude/skills/` and `skills/`). Use `python3 tools/release.py bump|check` for versions. Never commit credentials. Report test results honestly, including what could not be tested.
