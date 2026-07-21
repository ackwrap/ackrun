# 开始使用

## 使用前准备

需要准备：

- 一台使用 fw4/nftables、支持 TUN 的 OpenWrt 设备。
- 可以访问 Ackwrap Web 页面。
- 可以访问 GitHub Release 和自己的订阅地址，或者准备本地节点内容。
- 如果使用 TUN 模式，需要允许程序创建 TUN 网卡，并准备管理员或 root 权限。
- 如果使用 OpenWrt 透明代理，OpenWrt 应作为局域网客户端的默认网关和 DNS。

硬件最低建议为 1 个 CPU 核心、512 MB 内存和约 100 MB 可用空间。实际空间还要加上 sing-box、规则缓存和 Geo 数据库的大小。

## 安装 Ackwrap

### OpenWrt

使用项目提供的 Ackwrap IPK 安装包。可以在 LuCI 上传 IPK，也可以通过 SSH 执行：

```sh
opkg install ./ackwrap_<version>-1_<arch>.ipk
```

安装后可以在 LuCI 的“服务 → Ackwrap”中：

- 启用或停用 Ackwrap 服务。
- 设置监听端口。
- 查看或重新生成 API Token。
- 设置数据目录和日志选项。
- 点击“打开 Web 界面”。

默认数据目录为 `/etc/ackwrap`，其中的 `ackwrap.db`、`config`、`rules` 和 `geo` 目录不要随意删除。

首次安装会自动生成 API Token。通过 LuCI 打开 Web 界面时，系统会自动完成同主机认证，不需要把 Token 手动写进 URL。

OpenWrt 使用 TUN 透明代理前，还需要确认：

- 系统存在 `/dev/net/tun`。
- 系统使用 fw4/nftables。
- 安装了与当前内核匹配的 `kmod-nfnetlink-queue` 和 `kmod-nft-queue`。
- 已开启 IPv4 转发；需要代理 IPv6 时也开启 IPv6 转发。
- Ackwrap 的管理端口和代理端口不要暴露到 WAN 区域。

## 第一次访问

1. 启动 Ackwrap 服务。
2. 打开浏览器访问服务地址。
3. 如果浏览器出现 API Token 输入框，输入服务端配置的 Token。
4. 进入控制面板，等待运行状态和安装状态加载完成。

浏览器第一次验证成功后，前端使用 HttpOnly Cookie 保存登录状态。不要把 Token 保存到浏览器脚本可读的存储中，也不要分享给其他人。

## 远程访问安全

如果 Ackwrap 只监听 `127.0.0.1`，一般只允许本机访问，风险较低。

如果要让局域网或其他设备访问：

- 设置 `ACKWRAP_LISTEN_ADDR` 为需要监听的地址和端口。
- 同时设置足够长、随机的 `ACKWRAP_API_TOKEN`。
- 优先通过 VPN、SSH 隧道或 HTTPS 反向代理访问。
- 不要把管理端口、Mixed 代理端口或 Clash API 端口直接暴露到公网。

## 下一步

安装完成后，打开[控制面板](ControlPanel.md)安装 sing-box 核心并生成默认配置。
