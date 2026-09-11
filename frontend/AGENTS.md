# Frontend contracts

This file adds frontend contracts to the [root instructions](../AGENTS.md).

- Use the existing Vue 3, TypeScript, Vite, Tailwind CSS and DaisyUI stack.
  Check `package.json` and the lockfile for versions and available commands.
- Preserve the current blue-gray design and support both light and dark themes.
  Reuse components, `src/styles/global.css`, `tokens.css` and semantic tokens
  such as `var(--bg-*)`, `var(--text-*)` and `var(--border-*)`. Avoid page-local
  hardcoded dark colors for forms, dialogs, tables, buttons and notifications.
- Prefer dialogs for create/edit forms to retain list context and scroll position.
  Use inline forms only when the interaction has a concrete benefit. Keep desktop
  and mobile layouts usable and surface actionable API and sync failure messages.
- Split code by page, service and business component. Extract coherent business
  sections when responsibilities make the change difficult to understand or verify;
  file length alone does not require unrelated refactoring or one-line helpers.
- Node protocol/subscription counts come from `/api/v1/nodes/facets`, not the
  currently filtered page of nodes. Manual import stays under subscriptions;
  hide `manual://local` in the subscription list and use backend import previews.
- `npm run dev` uses port 5173 and proxies `/api` to the backend at port 8080.
  `npm run build` performs typechecking and writes the UI to
  `../backend/internal/webui/dist/` for Go embedding. Build artifacts are not source.
- Follow the [root verification matrix](../AGENTS.md#验证). Check changed Vue
  files with the installed Prettier and format only touched files when needed.
  For UI behavior changes, check the affected success/error interaction and
  desktop/mobile layout when a browser is available; otherwise report that gap.
