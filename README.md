# Ackwrap

[简体中文](./README.zh-CN.md)

Ackwrap is a local-first web console for managing a custom sing-box runtime. It helps organize subscriptions, nodes, node groups, strategy groups, route rules, DNS settings, Geo assets, and generated sing-box configuration in one place.

## Development Status

Ackwrap `v0.0.1`, the first public test release, is now available. The project remains in active development, and features, protocol mappings, sing-box compatibility details, OpenWrt workflows, and UI interactions are still being tested and refined. Expect breaking changes, incomplete behavior, and configuration edge cases before the project reaches a stable release.

Use it for experimentation and development first. Production use is not recommended yet.

The project is designed for iterative testing on desktop, Linux, and OpenWrt-like environments. It includes a Go backend, a Vue frontend, SQLite storage, WebSocket status events, and an installer that downloads Ackwrap's customized sing-box build from `ackwrap/sing-box-wrap` releases.

## Test Release

Download the latest public test release from [GitHub Releases](https://github.com/ackwrap/ackrun/releases/latest).

The current `v0.0.1` release provides OpenWrt x86_64 artifacts:

- `ackwrap_0.0.1-1_x86_64.ipk`: combined package containing the Ackwrap service, LuCI integration, and iStoreOS metadata.
- `ackwrap-openwrt-amd64`: standalone OpenWrt amd64 binary.

Back up existing Ackwrap data and configuration before upgrading. Test releases may introduce incompatible database, configuration, or runtime behavior changes.

## Features

- Subscription management for remote subscriptions and local/manual imports.
- Node parsing for common proxy formats including Clash YAML, sing-box JSON, base64 URI lists, and plain URI lists.
- Node management with filtering, flags, enable/disable, preference, TCP latency checks, emoji naming, and batch rename.
- Node groups and strategy groups for building selector/urltest/fallback outbounds from selected or filtered nodes.
- Route rule management with manual rules, rule subscriptions, Geo asset sync, and sing-box rule-set preview.
- DNS management for DNS servers, explicit real-IP rules, TUN FakeIP, and default leak protection with one proxied DNS final for non-direct traffic.
- Config generation with modular preview, full JSON preview, validation via `sing-box check`, and apply/reload flow.
- Realtime runtime, installer, core, config, and subscription status updates over WebSocket.
- Custom sing-box support through `ackwrap/sing-box-wrap`, including Ackwrap-specific VLESS encryption support.

## Tech Stack

- Backend: Go, Gin, SQLite (`modernc.org/sqlite`), Gorilla WebSocket, robfig/cron.
- Frontend: Vue 3, TypeScript, Vite, Vue Router, Tailwind CSS 4, DaisyUI.
- Runtime: sing-box-compatible JSON configuration.
- Storage: local SQLite database and filesystem cache under the Ackwrap data directory.

## Project Layout

```text
backend/                 Go backend and embedded frontend output
frontend/                Vue frontend
docs/                    Project documentation
sing-box-wrap/           Ackwrap-maintained sing-box fork/subtree
mihomo-Alpha/            Local reference source used for protocol research
```

## Development

Backend:

```bash
cd backend
go build ./...
go test ./...
go vet ./...
```

Frontend:

```bash
cd frontend
npm run build
```

Development servers:

```bash
cd backend && ACKWRAP_LISTEN_ADDR=127.0.0.1:8080 go run ./cmd/server
cd frontend && npm run dev
```

The frontend dev server runs on port `5173` and proxies API requests to the backend on port `8080`.

Release builds generate and embed the frontend, so no separate `ui/` directory is required:

```bash
# Build Windows, Linux, and OpenWrt amd64 artifacts into dist/
python build.py

# Build the OpenWrt arm64 binary and combined IPK package
python build.py --target openwrt --arch arm64
```

The OpenWrt target creates one architecture-specific `ackwrap` IPK containing the service, LuCI page, and iStoreOS metadata. Its UCI, procd, LuCI, and iStoreOS source templates live under `openwrt/`.

## Custom sing-box Build

Ackwrap downloads sing-box binaries from:

```text
https://github.com/ackwrap/sing-box-wrap/releases
```

The fork currently carries Ackwrap-specific changes such as VLESS encryption support through a forked `sing-vmess` dependency. Release artifacts use the `sing-wrap-*` naming scheme.

## References

Ackwrap is built with reference to the following projects and ecosystems:

- [SagerNet/sing-box](https://github.com/SagerNet/sing-box) - Core proxy runtime and configuration model.
- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo) - Clash-compatible proxy runtime and protocol behavior reference.
- [MetaCubeX/metacubexd](https://github.com/MetaCubeX/metacubexd) - Clash/Mihomo dashboard UX reference.
- [SagerNet/sing-geoip](https://github.com/SagerNet/sing-geoip) - GeoIP database source.
- [SagerNet/sing-geosite](https://github.com/SagerNet/sing-geosite) - GeoSite database source.
- [SagerNet/sing-box-dashboard](https://github.com/SagerNet/sing-box-dashboard) - sing-box dashboard ecosystem reference.
- [Dreamacro/clash](https://github.com/Dreamacro/clash) - Clash configuration conventions and historical ecosystem reference.
- [XTLS/Xray-core](https://github.com/XTLS/Xray-core) - VLESS/Reality protocol ecosystem reference.

## License

Ackwrap is released under the [MIT License](./LICENSE).

This repository may contain or reference third-party code and assets. Those components remain under their original licenses.
