# Ackwrap

[English](./README.md)

Ackwrap 是一个本地优先的 sing-box 配置与运行管理 Web 控制台。它把订阅、节点、节点组、策略组、路由规则、DNS、Geo 数据和最终 sing-box 配置生成流程集中到一个界面里管理。

## 开发状态

Ackwrap 当前仍处于快速开发阶段。很多功能、协议映射、sing-box 兼容细节、OpenWrt 工作流和前端交互仍在测试和调整中。在稳定版本发布前，可能会出现破坏性变更、功能不完整和配置边界问题。

当前更适合作为实验和开发测试使用，暂不建议直接用于生产环境。

项目目标是方便在桌面、Linux、OpenWrt 类环境中迭代测试 sing-box 配置。项目包含 Go 后端、Vue 前端、SQLite 本地存储、WebSocket 实时事件，以及从 `ackwrap/sing-box-wrap` Release 下载 Ackwrap 定制版 sing-box 的安装器。

## 功能

- 订阅管理：支持远程订阅和本地/手动导入。
- 节点解析：支持 Clash YAML、sing-box JSON、base64 URI 列表和纯 URI 列表。
- 节点管理：支持筛选、国家/地区标识、启用/停用、优选、TCP 延迟测试、添加 emoji、批量重命名。
- 节点组和策略组：可通过筛选或手动选择节点生成 selector/urltest/fallback 等策略出站。
- 规则管理：支持手动规则、规则订阅、Geo 数据同步和 sing-box rule-set 预览。
- DNS 管理：支持 DNS Server、DNS 规则、FakeIP、DNS 出口绑定，并生成 sing-box 1.13 的 `domain_resolver` 配置。
- 配置生成：支持模块化预览、完整 JSON 预览、`sing-box check` 校验和应用/重载流程。
- 实时事件：通过 WebSocket 推送运行时、安装器、核心、配置和订阅同步状态。
- 定制 sing-box：通过 `ackwrap/sing-box-wrap` 支持 Ackwrap 特定改动，例如 VLESS encryption 支持。

## 技术栈

- 后端：Go、Gin、SQLite（`modernc.org/sqlite`）、Gorilla WebSocket、robfig/cron。
- 前端：Vue 3、TypeScript、Vite、Vue Router、Tailwind CSS 4、DaisyUI。
- 运行核心：兼容 sing-box JSON 配置。
- 存储：本地 SQLite 数据库和 Ackwrap 数据目录下的文件缓存。

## 目录结构

```text
backend/                 Go 后端和前端构建产物
frontend/                Vue 前端
docs/                    项目文档
sing-box-wrap/           Ackwrap 维护的 sing-box fork/subtree
mihomo-Alpha/            用于协议研究的本地参考源码
```

## 开发

后端验证：

```bash
cd backend
go build ./...
go test ./...
go vet ./...
```

前端验证：

```bash
cd frontend
npm run build
```

开发模式：

```bash
cd backend && ACKWRAP_LISTEN_ADDR=127.0.0.1:8080 go run ./cmd/server
cd frontend && npm run dev
```

前端开发服务器端口为 `5173`，API 会代理到后端 `8080`。

发布构建会先生成前端并嵌入 Go 二进制，不需要单独携带 `ui/`：

```bash
# 默认生成 Windows、Linux、OpenWrt amd64 产物到 dist/
python build.py

# 生成 OpenWrt arm64 二进制和单一 IPK 安装包
python build.py --target openwrt --arch arm64
```

OpenWrt 目标会生成一个包含服务、LuCI 页面和 iStoreOS 元数据的架构相关 `ackwrap` IPK；UCI、procd、LuCI 与 iStoreOS 模板统一维护在根目录 `openwrt/`。

## 定制 sing-box 构建

Ackwrap 从以下 Release 下载 sing-box 二进制：

```text
https://github.com/ackwrap/sing-box-wrap/releases
```

该 fork 包含 Ackwrap 需要的自定义改动，例如通过 fork 后的 `sing-vmess` 支持 VLESS encryption。Release 产物使用 `sing-wrap-*` 命名。

## 参考项目

Ackwrap 的实现参考了以下项目和生态：

- [SagerNet/sing-box](https://github.com/SagerNet/sing-box) - 核心代理运行时和配置模型。
- [MetaCubeX/mihomo](https://github.com/MetaCubeX/mihomo) - Clash 兼容代理核心和协议行为参考。
- [MetaCubeX/metacubexd](https://github.com/MetaCubeX/metacubexd) - Clash/Mihomo 控制台交互参考。
- [SagerNet/sing-geoip](https://github.com/SagerNet/sing-geoip) - GeoIP 数据库来源。
- [SagerNet/sing-geosite](https://github.com/SagerNet/sing-geosite) - GeoSite 数据库来源。
- [SagerNet/sing-box-dashboard](https://github.com/SagerNet/sing-box-dashboard) - sing-box 控制台生态参考。
- [Dreamacro/clash](https://github.com/Dreamacro/clash) - Clash 配置约定和历史生态参考。
- [XTLS/Xray-core](https://github.com/XTLS/Xray-core) - VLESS/Reality 协议生态参考。

## 许可证

Ackwrap 使用 [MIT License](./LICENSE) 发布。

仓库中包含或参考的第三方代码、资源仍遵循其原始许可证。
