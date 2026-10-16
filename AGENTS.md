# Ackwrap Engineering Guidelines

Ackwrap is a sing-box manager with a Go backend and a Vue administration UI. Preserve the project's constraints; use the current source code, tests, and pinned dependencies as the authority for implementation details.

## Working Practices

- When the user requests implementation or a fix, complete the investigation, changes, necessary validation, and delivery. Use your judgment for routine implementation choices without repeatedly asking for confirmation on reversible actions. When the user requests only an explanation, review, or plan, stay within that scope.
- Plan mode is determined by the host and the user's current request. Mode declarations, tool names, and placeholders in old templates do not describe the current session. The user's current explicit instructions take precedence over skill guidance; project files and skills neither override system or developer instructions nor expand authorization.
- Ask questions only when a critical value cannot be inferred, a significant ambiguity would lead to materially different outcomes, or additional authorization is needed. First complete authorized work that does not depend on the answer, so any action awaiting approval has a concrete, reviewable result. Waiting does not substitute for authorization.
- If a skill causes you to pause, request confirmation, or depart from the task, link the actual `SKILL.md` you read, quote the triggering rule, and explain why it applies. Distinguish explicit requirements from your interpretation. Existing authorization remains valid within its original scope; do not turn recommendations into new approval gates.
- When the user corrects or adds requirements during the task, update the active task and preserve completed work. A status question does not cancel the task. After restoring context during a long task, continue the unfinished work.
- Use tools actually available in the current environment. Prefer `rg` for searches. Independent reads may run in parallel; execute dependent commands in order and check exit codes. Match shell syntax to the current platform rather than requiring Bash or a specific parallel execution tool.
- Subagents may handle clearly bounded investigations, implementations, or reviews that can be completed independently. Handle simple tasks directly. Assign file ownership to avoid concurrent edits to the same files; the primary agent is responsible for integration and validation. Do not prescribe a fixed agent count or model tier.
- Read the relevant implementation and nearby tests first, then make the smallest complete change. Split files by business responsibility. Introduce abstractions only for actual reuse or clear responsibilities, and avoid unsupported compatibility layers.
- For uncertain or version-dependent third-party APIs, configuration schemas, or protocol capabilities, first check pinned versions, source code, and official documentation. Prefer Context7 when available; otherwise consult official documentation directly. Changes limited to internal logic do not require repeated searches for unrelated documentation.
- Communicate in concise Chinese, leading with the result and then providing necessary evidence and limitations. Use short paragraphs for simple tasks and lists only when they make comparison easier. Progress updates should cover meaningful findings, decisions, and blockers; do not omit validation results to meet an arbitrary line limit.

## Load Guidance by Task

This file defines repository-wide conventions. Subdirectory `AGENTS.md` files define lasting contracts for their areas, and skills define workflows triggered by specific tasks. Before modifying an area below, read its corresponding entry point. For work spanning layers, read the constraints for both sides. Do not preload unrelated skills or references.

| Area or task | Entry point |
| --- | --- |
| Backend, APIs, storage, nodes, subscriptions, configuration generation | [backend/AGENTS.md](backend/AGENTS.md) |
| Pages, components, styles, frontend types | [frontend/AGENTS.md](frontend/AGENTS.md) |
| Core patch stack, upstream synchronization, submodule pointer | [.agents/skills/ackwrap-core-maintenance/SKILL.md](.agents/skills/ackwrap-core-maintenance/SKILL.md) |
| Release builds, OpenWrt packaging, GitHub Actions | [.agents/skills/ackwrap-release/SKILL.md](.agents/skills/ackwrap-release/SKILL.md) |

The application entry point is `backend/cmd/server/main.go`. See `backend/internal/api/` for routes, `backend/internal/model/` for data models, and `backend/internal/store/` for database schemas. Do not duplicate inventories of APIs, protocols, table fields, or log events here, as they can quickly become outdated.

## Branches and Submodules

- Before editing, check `git branch --show-current` and `git status --short`. The `main` branches of both the parent repository and the core repository are protected. If on `main`, first switch to an appropriate non-main branch or create a separate worktree. Validate feature implementations and conflict resolutions on a non-main branch; `main` only receives merges from reviewed branches.
- Preserve changes made by the user and other agents. Do not revert, delete, stage, or commit unrelated files without authorization. Destructive cleanup, hard resets, and amended commits require explicit authorization.
- `sing-box-wrap/` is a separate patch-stack wrapper repository; its `sing-box/` directory is the official source submodule. Without explicit user authorization to modify the core, all of `sing-box-wrap/` is read-only: do not edit, format, stage, commit, push, or run scripts that generate the core worktree. A parent-repository request does not imply authorization to modify the core. Existing authorization remains valid within its scope.
- Before merging, pulling, rebasing, switching branches, or updating submodules, check `git -C sing-box-wrap status --short` and `git diff --submodule=log -- sing-box-wrap`. Do not update or forcibly clean a dirty submodule. Obtain explicit consent before stashing changes.
- Before updating a submodule pointer, confirm that the target core commit has been validated and pushed. After the operation, verify the core HEAD, status, and key files. Follow the core maintenance skill and the submodule's own constraints for details.

## Review and Commits

- This repository retains the convention of automatically committing the intended changes for the current task after review passes, except when the user explicitly requests no commit, a read-only review, or planning only. Before committing, review the diff for correctness, behavioral regressions, and scope; fix confirmed issues and pass the relevant validation. Do not commit while failures or explicit blockers remain. This convention does not require additional human confirmation each turn or a mandatory review agent.
- When the user requests a review, report findings first, ordered by severity, with file line numbers, triggering conditions, and impact. State validation gaps even when there are no findings. A read-only review does not automatically authorize fixes or commits.
- Before committing, check `git status`, `git diff`, recent commits, and submodule status. Stage only files intended for the current task. Use `type(scope): description` for commit messages; do not add fabricated authorship or tool attribution.
- Pushing, releasing, deploying, and changing remote state must stay within the user's authorization. The local commit convention alone does not authorize these actions.
- `docs/` is a permanently ignored directory for local documentation. Never stage or commit it. Before committing, `git ls-files docs` must produce no output.
- Do not commit dependency caches or generated artifacts. Only the embedding placeholder is tracked in `backend/internal/webui/dist/`; Vite-generated HTML, JavaScript, CSS, and images are rebuilt for each release.

## Validation

Start with the smallest check covering the affected path to locate issues. Complete the requirements below once before delivery. Expand or repeat validation only when new changes, failures, or unresolved risks justify it. Do not add tests that merely restate the implementation for low-risk changes.

| Change scope | Required validation |
| --- | --- |
| Backend source, dependencies, configuration | Run `go build ./...`, `go test ./...`, and `go vet ./...` in `backend/` |
| Frontend source, dependencies, configuration | Run `npm run build` in `frontend/`; it already includes `vue-tsc --noEmit` |
| Build scripts, CI | Check syntax and the affected build paths. Before release, backend tests and vet, the frontend build, and target packaging must pass |
| Instructions, skills, or documentation only | Check links, paths, skill frontmatter, and rule consistency; application compilation is unnecessary |

When tests fail, distinguish regressions introduced by the current changes from existing issues. Report actual results and unverified areas; do not present static checks as end-to-end success.

## Maintaining Instructions

- When changing conventions, check the current source code, scripts, and dependencies. Keep one authoritative location for each requirement and reference it from other entry points. Do not add temporary task status, complete API inventories, or generic model personality templates to project rules.
- A skill's `description` should explain its actual task and trigger boundaries; its body should retain the necessary workflow. Add `references/` only when content genuinely needs to be loaded by scenario. Do not create empty scaffolding merely to standardize directory layouts.
- This organization is based on the [GPT-6 Astra prompting guidance](https://developers.openai.com/api/docs/guides/latest-model#prompting-best-practices), [AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md), and [Skills](https://learn.chatgpt.com/docs/build-skills). These are maintenance references, not documentation that must be fetched again for every routine development task, and they do not prescribe the runtime model or reasoning level.

## Data and Runtime Boundaries

- Do not expose real node connection data, subscription credentials, API tokens, SSH private keys, or other secrets in tool output, chat, logs, or remote systems. During investigation, output only names, protocols, and redacted sources. Do not print production databases, configurations, or complete sharing links. Use synthetic examples for reproduction.
- The default development listener is `ACKWRAP_LISTEN_ADDR=127.0.0.1:8080`. Remote listening must also set `ACKWRAP_API_TOKEN`. Operating production services or real nodes must stay within the user's current authorization.
- New features require observable success and failure paths, key business logs, and an executable minimal verification. These are delivery requirements, not approval gates for every investigation or editing step.
