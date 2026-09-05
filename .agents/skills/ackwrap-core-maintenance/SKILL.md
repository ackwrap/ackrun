---
name: ackwrap-core-maintenance
description: Investigate or maintain Ackwrap's sing-box-wrap patch stack, upstream pin, or parent gitlink. Use for core sync and patch rebase tasks, not ordinary backend or frontend work.
---

# Core maintenance

Read `sing-box-wrap/AGENTS.md`, its `README.md` and the affected patch metadata
before choosing a workflow. Paths below are relative to the Ackwrap root unless
the command specifies another working directory.

## Establish the boundary

- Read-only investigation does not require core mutation permission. Editing any
  wrapper/core file, preparing `.work/`, or changing its checkout requires the
  user's explicit core authorization. Existing authorization remains valid within
  its stated scope; do not ask again for already-authorized steps.
- If mutation is needed but unauthorized, identify the reason, target files,
  behavioral impact and viable parent-only alternative. Finish independent parent
  work before requesting that decision. Do not interpret this skill as approval.
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

- Recreate the patched source from clean inputs. In the prepared tree, verify
  `go mod tidy` leaves `go.mod`/`go.sum` unchanged, then run `make build` and
  `go build ./cmd/sing-box`. Run affected tests required by the wrapper's current
  policy; do not weaken upstream tests to obtain a pass.
- Before updating the parent's gitlink, the intended wrapper commit must be
  validated, committed and available remotely. Push only within user authorization.
  Verify any newly pinned nested commit is also reachable by a clean clone.
- After the operation, compare wrapper HEAD, statuses, patch list and key files
  with the intended result. Report the verified commits and any untested behavior.
