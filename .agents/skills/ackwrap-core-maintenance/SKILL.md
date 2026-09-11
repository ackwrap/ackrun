---
name: ackwrap-core-maintenance
description: Investigate or maintain Ackwrap's sing-box-wrap patch stack, upstream pin, or parent gitlink. Use for core sync and patch rebase tasks, not ordinary backend or frontend work.
---

# Core maintenance

Use this workflow for core investigation, patch maintenance or upstream updates.
Read `sing-box-wrap/AGENTS.md`, then the relevant sections of its `README.md`
and affected patch metadata. Paths below are relative to the Ackwrap root unless
the command specifies another working directory. Shared authorization, branch,
commit and data rules live in [AGENTS.md](../../../AGENTS.md).

## Establish the boundary

- Read-only investigation does not require core mutation permission. Editing any
  wrapper/core file, preparing `.work/`, or changing its checkout requires the
  user's explicit core authorization. Existing authorization remains valid within
  its stated scope; do not ask again for already-authorized steps.
- If mutation is needed but unauthorized, finish read-only diagnosis and independent
  parent work first. Present the target files, behavioral impact and any viable
  parent-only alternative, citing this boundary when requesting authorization.
  A request to investigate or audit remains read-only; do not prepare `.work/`
  merely to make an unauthorized change reviewable.
- Record parent and wrapper branches, HEADs, status and parent gitlink diff.
  Check the nested official submodule status too. Follow the root branch and
  dirty-worktree protections before any checkout, merge or update.

## Work on the current patch-stack design

- `sing-box-wrap/sing-box/` is pinned official source; never edit it directly.
  Production customization lives in ordered patches from `patches/series`.
  The nested gitlink and `patches/upstream.txt` must identify the same commit.
- Once authorized, run `python scripts/prepare_core.py` from `sing-box-wrap/`
  to prepare disposable `.work/sing-box` source. Check the script's overwrite
  behavior and preserve any existing work before using its output directory.
- Edit disposable patched source, export only the intended production diff to
  the relevant feature patch, and retain patch order. Do not commit `.work/`.
  Observe the wrapper's current restriction on adding test patches.
- For upstream updates, compare old base, target upstream and the custom patch
  intent. Adapt useful upstream changes while retaining required custom behavior;
  do not blindly choose whole files or apply the retired `sync -> devel` fork flow.
  Record each conflict decision and its regression implications.

## Verify and deliver

- For read-only investigation, report the inspected commits, evidence and limits;
  the build and mutation steps below apply only to authorized maintenance.
- Recreate the patched source from clean inputs. In the prepared tree, verify
  `go mod tidy` leaves `go.mod`/`go.sum` unchanged, then run `make build` and
  `go build ./cmd/sing-box`. Run affected tests required by the wrapper's current
  policy; do not weaken upstream tests to obtain a pass.
- Before updating the parent's gitlink, the intended wrapper commit must be
  validated, committed and available remotely. Push only within user authorization.
  Verify any newly pinned nested commit is also reachable by a clean clone.
- After the operation, compare wrapper HEAD, statuses, patch list and key files
  with the intended result. Report the verified commits and any untested behavior.
- If publishing the wrapper commit is not authorized, leave the parent's gitlink
  unchanged and report the remaining step. Local verification is not evidence
  that another checkout can fetch an unpublished commit.
