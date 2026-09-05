# Frontend contracts

- Use the existing Vue 3, TypeScript, Vite, Tailwind CSS and DaisyUI stack.
  Check `package.json` and the lockfile for versions and available commands.
- Preserve the current blue-gray design and support both light and dark themes.
  Reuse components, `src/styles/global.css`, `tokens.css` and semantic tokens
  such as `var(--bg-*)`, `var(--text-*)` and `var(--border-*)`. Avoid page-local
  hardcoded dark colors for forms, dialogs, tables, buttons and notifications.
- Prefer dialogs for create/edit forms to retain list context and scroll position.
  Use inline forms only when the interaction has a concrete benefit. Keep desktop
  and mobile layouts usable and surface actionable API and sync failure messages.
- Split code by page, service and business component. If an edited source file
  exceeds 800 lines, extract coherent business sections rather than adding to it;
  avoid one-line helpers created only to reduce the count.
- Node protocol/subscription counts come from `/api/v1/nodes/facets`, not the
  currently filtered page of nodes. Manual import stays under subscriptions;
  hide `manual://local` in the subscription list and use backend import previews.
- `npm run dev` uses port 5173 and proxies `/api` to the backend at port 8080.
  `npm run build` performs typechecking and writes the UI to
  `../backend/internal/webui/dist/` for Go embedding. Build artifacts are not source.
- Run `npm run build` once after frontend changes. For changed Vue formatting,
  use the installed Prettier on the touched files; avoid formatting unrelated files.
  Pure instruction changes do not require application builds.
