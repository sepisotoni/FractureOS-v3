---
name: release-prep
description: Use whenever you bump a version, prepare, tag, package or publish a FractureOS release, or touch the updater, installer, SHA256SUMS or changelog. Enforces docs/RELEASE-PROTOCOL.md.
---

# FractureOS release prep

1. Read `docs/RELEASE-PROTOCOL.md` and follow it in order. It is the source of truth.
2. Change the version only with `python3 tools/release.py bump <X.Y.Z[.W]>`; then add the `host/changelog.go` entry; then run `python3 tools/release.py check`. Never hand-edit version strings.
3. Run every verification command in section 2 of the protocol. Report each as pass, fail or not testable, with the reason. Do not claim a result you did not run.
4. Never move or re-tag a published tag; ship a new version instead.
5. A release is not finished until `SHA256SUMS` lists both Windows setup exes and the previous version's in-app update has been checked (protocol section 4), or you have said plainly that you could not check it.
6. Never write tokens or secrets anywhere in the repo, commits or `.git/config`.
