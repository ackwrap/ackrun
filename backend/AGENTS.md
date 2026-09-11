# Backend contracts

This file adds backend contracts to the [root instructions](../AGENTS.md).
Dependency versions are defined by `go.mod` and `go.sum`; API routes and schemas
are defined by the source. Read only the implementation relevant to the task.

## Architecture

- Keep the existing Go, Gin, SQLite (`modernc.org/sqlite`), Gorilla WebSocket,
  and `robfig/cron/v3` stack. Do not introduce GORM, Redis, queues, a DI framework,
  or another database without a concrete requirement.
- `handler` parses requests and returns API responses; `service` owns business
  logic, logging and orchestration; `store` owns SQL; `model` contains data types;
  `parser` parses subscriptions and protocols without HTTP or database access.
- Register routes in `internal/api/`; assemble dependencies in `cmd/server/`.
  Handlers must not access SQL or execute sing-box; services do not return Gin
  responses. Split business code by feature and protocol parsers by protocol.
- Use `internal/paths.Paths` for filesystem locations. Preserve environment
  overrides, legacy config migration, active config selection and backup exclusion.
  Defaults and markers are maintained in `internal/paths/paths.go`.
- Put DDL in `internal/store/migrations.go`. Add columns with the existing
  `ALTER TABLE` convention and ignore only allowlisted duplicate-column errors
  through `isDuplicateColumnMigration()`; preserve existing user data.

## API and observability

- REST uses `/api/v1`. Errors retain
  `{ "error": { "code": "STRING_CODE", "message": "...", "details": {} } }`.
  Return resources directly on success, or `{ "success": true, "message": "..." }`
  for actions. Preserve existing API contracts when extending a feature.
- The realtime channel is `/api/v1/realtime/ws`. Events contain `type`, millisecond
  `time`, and a stable `data` payload. Use `module.event` names. REST starts actions;
  WebSocket reports progress and final state. A failed `subscription.sync` includes
  `error` so the UI can show the cause.
- Use `internal/logging` for key actions, state transitions, sync and scheduler
  failures, with the existing `module.action` names. Never log connection secrets.

## Subscriptions and nodes

- Preserve the formats supported by the existing parser pipeline. Fetch with
  the saved user agent and sync timeout.
- Parse, apply enabled backend filters, then write. Zero parsed nodes or zero
  remaining nodes is a failed sync that preserves the previous nodes. Validate
  Go regular expressions on filter writes; previews use the same backend pipeline.
- Replace subscription nodes by stable UID and update usage, expiry, count and
  sync state. UID is a short SHA-256 hash of the connection-field whitelist;
  names, tags, raw text, status, IDs and timestamps must not affect it.
- Inherit `enabled`, `preferred`, `latency_ms`, `status` and overridden names by
  UID. Rename and emoji actions set `name_overridden=1`; later sync must preserve
  the custom name. Prefer disabling subscription nodes over deleting them.
- Manual imports use `manual://local`, append/update by UID and retain older
  manual nodes. `SyncAll()` skips this source. Apply the same parsers and enabled
  filters to manual import and preview as to subscription sync.
- Maintain cron jobs on create/update/delete. Use `cron.WithSeconds()` for the
  existing six-field expressions, daily time and weekly weekday, and the configured
  `ACKWRAP_TIMEZONE`. Creation and URL changes trigger an asynchronous sync.
- Preserve the distinction between core delay probes and TCP probes. Determine
  protocol selection from the current probe implementation. Core probes require
  nodes in the active config; missing nodes fail without falling back to TCPing.
  TCP probes use `net.JoinHostPort` for IPv6 and prove only TCP reachability.
  Success stores latency and `available`; failure stores zero and `unavailable`.

## Routing and core configuration

- Node filters are separate from routing rules. Rule preview converts enabled
  manual rules and rule subscriptions to `route.rules` and `route.rule_set`.
- Cache rule subscriptions through `Paths.RulesDir`. The content endpoint serves
  cache first and fetches upstream when absent; upstream requests honor `use_proxy`.
  Infer auto format from URL suffix and convert Clash YAML `payload`/`rules`,
  classical lines, plain domains and CIDRs through the existing converter.
- Generated remote rule sets use the local cache/conversion endpoint. Keep its
  direct HTTP client routing and version-specific schema handling; consult current
  code before adding removed fields such as `download_detour`.
- Rule subscription creation and URL/format/proxy changes trigger async sync.
  Maintain manual and daily/weekly sync for rule subscriptions and Geo assets,
  including observable failures. Geo assets use `Paths.GeoDir`.
- Write generated configs to a temporary file, run the selected core's
  `sing-box check -c <temporary-config>` and replace the active file only on
  success. A failure must preserve the working config and reach the caller/UI.
- Preserve process and enabled-node direct bypass rules ahead of user rules,
  with process lookup enabled, to prevent TUN/global proxy loops. Match node
  domains as `domain`, IPv4 as `/32` and IPv6 as `/128`; never log the addresses.
- Diagnose `unknown outbound type` against the actual binary version/build tags,
  wrapper patch stack, upstream `constant/`, `option/`, `include/` registration
  and Ackwrap's generator/tests. Core support and standalone node support differ.
  Do not invent an unsupported-protocol list or silently skip a protocol merely
  because it is called third-party; use version-matched official documentation
  and a synthetic config to establish the supported behavior.

## Verification

Follow the [root verification matrix](../AGENTS.md#验证). Choose focused tests
for changed contracts, especially sync failure preserving nodes, UID inheritance,
migrations preserving data, and invalid configuration preserving the active file.
Exercise only the cases affected by the task; this is not an additional full-suite gate.
