# Ackwrap 使用指南

本文根据当前 Ackwrap Web 界面和项目文档整理，帮助用户完成安装、配置、运行和日常维护。

## 文档导航

- [开始使用](GettingStarted.md)：安装、首次访问、权限和远程访问安全。
- [控制面板](ControlPanel.md)：安装核心、启动服务、运行模式和高级维护。
- [订阅与节点](SubscriptionsAndNodes.md)：订阅同步、手动导入、节点筛选和测速。
- [策略组](StrategyGroups.md)：节点组、策略组和自动测速选择。
- [规则与 DNS](RulesAndDns.md)：路由规则、规则订阅、Geo 数据库和 DNS。
- [配置与监控](ConfigurationAndMonitoring.md)：配置生成、仪表盘和日志。
- [设置](Settings.md)：连通性测速、NTP、流量排除、外部控制面板和更新。
- [故障排查与维护](TroubleshootingAndMaintenance.md)：常见问题、备份、升级和安全建议。

## Ackwrap 能做什么

Ackwrap 是用于管理 sing-box 的 Web 平台，可以集中管理订阅、节点、路由规则、DNS 和核心进程。

- 安装和更新 Ackwrap 定制版 sing-box 核心。
- 添加多个代理订阅，并手动或定时同步。
- 查看、筛选、测速、启用或禁用节点。
- 导入单个节点或整段节点内容。
- 创建节点组和代理策略组。
- 管理手动路由规则、规则订阅和 Geo 数据库。
- 管理 DNS Server、DNS 规则和 TUN 模式下的 FakeIP。
- 自动生成、校验、备份和应用 sing-box 配置。
- 启动、停止、重启核心，查看实时日志和运行状态。

Ackwrap 不提供代理订阅，也不会自动产生节点。使用前需要准备自己拥有或有权使用的订阅链接，或者准备节点 URI、Clash YAML、sing-box JSON 等内容。

## 推荐使用顺序

1. 阅读[开始使用](GettingStarted.md)，完成安装并打开 Web 页面。
2. 在[控制面板](ControlPanel.md)安装 sing-box，并生成默认配置。
3. 在[订阅与节点](SubscriptionsAndNodes.md)添加订阅并等待同步完成。
4. 在节点列表中测速，禁用不可用节点，必要时标记首选节点。
5. 按需阅读[策略组](StrategyGroups.md)和[规则与 DNS](RulesAndDns.md)，配置分流策略。
6. 在[配置与监控](ConfigurationAndMonitoring.md)中生成、校验并应用完整配置。
7. 返回控制面板启动核心，再通过仪表盘观察运行情况。

配置相关数据发生变化后，Ackwrap 可能自动生成、校验并应用配置。第一次使用仍建议主动打开配置生成页面确认最终结果。

## 常用名词

| 名词 | 简单理解 |
|---|---|
| 订阅 | 一条可以返回多个节点的链接，通常由服务商提供 |
| 节点 | 一个具体的代理服务器 |
| 节点组 | 按协议、订阅或关键词筛选出的一批节点 |
| 策略组 | 给某类流量使用的代理方式，可以手动选节点，也可以自动测速选最快节点 |
| 路由规则 | 判断某个域名、IP 或规则集应该走代理、直连还是阻断 |
| DNS Server | 负责把域名解析成 IP 的服务器 |
| Mixed 模式 | 提供 HTTP 和 SOCKS5 代理端口，需要客户端手动填写代理地址 |
| TUN 模式 | 创建虚拟网卡，接管系统或局域网流量 |
| TUN + Mixed | 同时提供 TUN 虚拟网卡和 Mixed 代理端口 |
| 规则模式 | 按路由规则决定代理或直连 |
| 全局模式 | 默认所有流量走代理 |
| 直连模式 | 默认所有流量直连 |
