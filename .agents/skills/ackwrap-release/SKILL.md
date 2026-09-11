---
name: ackwrap-release
description: Build or audit Ackwrap release artifacts, OpenWrt IPK packaging, or the GitHub Actions release workflow. Use for build.py, build_release.py, openwrt, or release CI changes; not ordinary application edits.
---

# Release workflow

Choose the workflow from the requested outcome: audit, local artifact build,
CI change or publication. An audit stays read-only; a local build does not imply
publication. Commands below run from the repository root. Shared branch,
commit and artifact rules live in [AGENTS.md](../../../AGENTS.md).

Read only the relevant entrypoints:

- `build.py` for local build and packaging mechanics; `openwrt/` for package files.
- `.github/workflows/build-openwrt.yml` for CI changes or release gate checks.
- `build_release.py` and the workflow before dispatching a release.

## Local build

- `python build.py` is the release entrypoint. It builds the frontend, runs
  backend tests/vet and produces Windows, Linux and OpenWrt amd64 outputs.
- For a selected target use, for example,
  `python build.py --target openwrt --arch amd64 --version <version>`.
  OpenWrt also supports `arm64`. Verify CLI choices in `build.py` before use.
- The frontend uses `npm ci` if dependencies are absent and `npm run build`
  embeds fresh assets into the backend. In a reused checkout, synchronize
  dependencies when the lockfile changed; do not rely on stale `node_modules`.
- `--skip-checks` is appropriate only when the same source's backend tests and
  vet already passed or a required CI dependency gates publication on them.
  Target builds and frontend typechecking/build must still succeed.
- OpenWrt outputs one IPK combining the Ackwrap daemon, LuCI and iStoreOS app metadata.
  Verify archive members, architecture, executable modes, version and expected
  binary names when packaging changes. Do not install on a router to test an archive.

## CI and publication

- Keep manual dispatch, version validation, SHA-pinned actions, bounded job
  timeouts and read-only build permissions. Pass user inputs through environment
  variables into quoted shell arguments, not directly into executable shell text.
- The amd64 matrix entry runs backend tests/vet; arm64 reuses that release gate
  while cross-compiling its own artifact. Publication depends on all build entries
  succeeding and runs only for `main`.
- Validate workflow structure and embedded shell when changing CI. Exercise the
  affected build path locally when feasible. Distinguish local verification from
  a successful GitHub-hosted run; never dispatch a release just to validate YAML.
- Publication needs explicit release authorization; reuse authorization already
  given for the same scope. Prepare authorized checks and resolve the version,
  commit and expected assets before asking for any missing publication approval.
  Cite this boundary when approval is required. `build_release.py` dispatches on
  `main` and may publish.
  Preserve the existing checks for a tag pointing elsewhere and an existing release;
  do not overwrite published assets or move a tag as a routine retry.
- Check final artifact paths and report build/check results. Keep `dist/` and
  generated embedded assets out of commits. Parent releases do not authorize
  modification or publication of the separate sing-box-wrap core.
