# OpenWrt 流量卸载集成回归

基于 ackrun v0.3.5（PR #7，`3885c64f096361f5758ab126a0397959d87866c8`）。复用
`backend/internal/service/core_transition_guard_integration_test.go` 的网络拓扑、
IPv4/IPv6 TCP/UDP 服务器、已有连接及卸载旁路证明，增加 `openwrt-real-fw4` 模式。
该模式使用安装在 OpenWrt 内的 `/sbin/uci`、`/sbin/fw4`、`nft` 和
`/proc/net/nf_conntrack`，不替换它们的成功响应或连接状态。

## 验证状态

本次开发环境没有 Docker/QEMU、网络命名空间权限或任何 Linux capabilities，
`unshare -n true` 返回 `Operation not permitted`。**这里没有运行 OpenWrt 集成测试，
也没有验证生产路由器升级。** 本地完成了以下检查：

- `npm ci`、`npm run build`（生成应用测试所需的嵌入 UI，不提交产物）。
- `backend/` 中的 `go build ./...`、`go test ./...`、`go vet ./...`。
- 卸载/切换保护相关定向回归、静态 amd64 测试二进制编译和执行。
- 三个 shell 脚本的语法检查，以及 runner 在非 OpenWrt 环境下拒绝执行。

这些检查只证明相应范围，不能填成下面矩阵的通过结果。

本次还确认并修复了一个确定性问题：卸载恢复记录损坏时，原 `Disable()` / 强制解除
路径先删除防泄漏表及范围记录，再报恢复错误。现在先校验恢复记录，失败时保持保护。
`TestFW4OffloadCorruptRecordDoesNotReleaseGuard` 在修复前失败，在修复后通过；这属于
单元回归证据。`corrupt-record` 场景将同一要求交给真实 OpenWrt 工具和数据包检验。

## 一次性 VM

只使用没有生产配置的 x86/64 OpenWrt QEMU/KVM 虚拟机。不要复制真实路由器的配置、
订阅或密钥，也不要在实际开发防火墙上直接执行测试。容器需自行提供匹配内核能力；
本套件默认拒绝没有 QEMU/KVM DMI 标识的主机。

准备 `generic-ext4-combined.img`（BIOS，解压后的 raw 磁盘），记录镜像下载地址、
OpenWrt 源码/发行版本和 SHA-256。可从 [OpenWrt 官方下载](https://downloads.openwrt.org/)
选择固定版本。镜像应预装以下组件，测试时不联网安装依赖：

- `firewall4`、`uci`、`nftables-json`、`ubusd`；保留 OpenWrt 自带的 ucode 模块。
- `ip-full`、`unshare`、`nsenter`、`mount-utils`、`python3`。
- `kmod-veth`、`kmod-nft-offload`、IPv6 和正常的 conntrack/nftables 模块。
- 内核支持 network/mount namespaces、sysfs 和 `CONFIG_NF_CONNTRACK_PROCFS=y`。
  如果发行镜像缺少能力，应重建测试镜像；不能以跳过断言或模拟 conntrack 代替。

源码构建时启用 `CONFIG_KERNEL_NAMESPACES=y`、`CONFIG_KERNEL_NET_NS=y`，并选择上述
软件包；内核 conntrack procfs 开关以该版本实际内核配置为准。包名及 namespace 配置
可核对 [util-linux 包定义](https://github.com/openwrt/openwrt/blob/openwrt-24.10/package/utils/util-linux/Makefile)
和 [内核配置](https://github.com/openwrt/openwrt/blob/openwrt-24.10/config/Config-kernel.in)。

宿主机执行（默认使用 TCG，无需 KVM 权限）：

```sh
./openwrt/tests/run-qemu.sh /absolute/path/openwrt-generic-ext4-combined.img 22222
```

脚本使用 QEMU 临时磁盘快照、仅回环地址 SSH 转发和用户态管理网络，没有宿主机目录
共享或宿主机防火墙操作。磁盘更改随 VM 退出丢弃。镜像的管理 LAN 应为默认
`192.168.1.1/24`，SSH 认证用测试镜像自有的账号/密钥；磁盘至少为测试包和日志预留
300 MiB。若镜像使用不同管理地址，先在镜像准备阶段调整它，不要改接真实路由器。

## 编译和运行

在仓库根目录使用 `backend/go.mod` 指定的 Go 工具链：

```sh
./openwrt/tests/build-offload-integration.sh
scp -P 22222 -r dist/openwrt-integration root@127.0.0.1:/root/
```

编译包包含静态 Linux amd64 `service.test`，VM 无需 Go 或整个仓库。正式留档前应
先提交所有测试源码；`source-commit.txt` 记录 HEAD，`source.patch` 记录已跟踪源码
相对 HEAD 的差异，`SHA256SUMS` 标识实际运行的测试二进制和 runner。

在 **VM 控制台或它的回环 SSH 连接** 中：

```sh
printf '%s\n' ackrun-openwrt-integration-v1 > /etc/ackrun-disposable-vm
cd /root/openwrt-integration
sha256sum -c SHA256SUMS
./run-offload-integration.sh
```

runner 再创建独立 mount/network namespace，绑定空白 `/etc/config` 和 `/tmp`，
挂载属于新网络空间的 sysfs，启动独立 ubusd。fw4 zone 显式引用测试 veth，
不用 VM 管理接口的 netifd 状态。`auto_includes=0` 避免加载镜像里的用户 include。
所有 UCI delta、fw4 lock/state、nft 表、路由及 sysctl 修改都在这个隔离范围内。
二进制和结果位于 `/root`，不能放入会被隐藏的 `/tmp`。

测试退出后对比外层 VM 的 firewall 文件、待提交 UCI 修改和无状态 nft ruleset。
任何差异、缺失能力、跳过或缺失预期子测试都会失败。信号/超时使 runner 非零退出，
不应把残留日志中的单项 PASS 当作整个套件通过。

## 通过标准

下面每一行都必须满足；缺少工具或内核能力是失败/未验证，不是通过。

| 场景 | 必须满足的结果 |
| --- | --- |
| 已卸载 IPv4/IPv6 TCP/UDP | 四条已建立连接预热；插入 fw4 forward drop 后仍能回显，证明真的走了卸载；暂停后同一 socket 不能回显 |
| 暂停和恢复 | 原始 UCI 记录先于写操作存在；两项卸载按原始设置暂停；真实 fw4 reload 后没有 flowtable 和 OFFLOAD/HW_OFFLOAD 连接；解除后恢复设置及 fw4 flowtable，删除恢复记录 |
| 设备范围和防泄漏 | 选中设备的新建及已有 TCP/UDP、公网 DNS、路由器本地 DNS 和 FakeIP 被阻断；其他设备、路由器输出及本地非 DNS 管理保持可用；独立防护表跨 fw4 reload 保留 |
| `pending-before` / `pending-during` | 开始前/私有 staging 过程中出现的真实共享 UCI delta 导致拒绝；磁盘原配置逐字节不变；其他人的 delta 不被提交或丢弃 |
| `committed-unrelated` | staging 时另一个 UCI 客户端提交 `synflood_protect`；暂停及恢复均保留此修改，并且其余非卸载设置不变 |
| `committed-offload` | staging 时另一个客户端提交卸载设置；拒绝覆盖，保留该值 |
| `reload-pause` | fw4 check 后加入无效 nft include；真实 fw4 reload 返回失败；保留原始恢复记录；移除故障后可重新暂停、阻断、恢复 |
| `reload-restore` | 已就绪解除阶段加入无效 nft include；真实 reload 失败且记录保留；修复输入后新的解除调用完成恢复 |
| `corrupt-record` | 损坏卸载恢复记录后解除失败；原防护表、范围记录及丢包行为保留；恢复原记录后可正常恢复 |
| `permanent-policy` | 使用生产规则生成器安装永久设备防护；临时保护解除后仍暂停卸载、保留记录；fw4 reload 后数据包仍被永久防护拦截；移除永久策略后才恢复卸载 |
| `kill-intent` / `kill-commit` / `kill-reload` / `kill-guard` | 在持久记录已写、UCI 已提交、fw4 已 reload、保护表已装四个边界 SIGKILL 真进程；无共享 UCI delta 污染；新实例读取同一记录，恢复暂停及防护，再恢复原设置；`kill-guard` 在进程死亡后仍阻断 |
| 无关状态 | 除两个卸载选项及显式注入的修改外，完整 UCI export 不变；无关 sentinel nft 表跨 fw4 reload 保留；外层 VM firewall/UCI/nft 完全不变 |

暂停失败时旧核心不应停止，解除只能在新核心就绪且更新提交完成后发生；这一生命周期
约束继续复用现有 `TestTransitionGuardReleaseRequiresReadyCoreAndUpdateCommit` 和
installer 回归。本套件没有安装真实 sing-box 或运行 IPK 升级，因此不能据此声称
真实核心就绪探测、IPK 升级、断电重启、硬件卸载芯片或所有 OpenWrt 版本已经验证。
恢复时的 fw4 reload 故障注入发生在调用方已声明核心就绪的解除阶段。

并发 UCI 用确定的 staging 边界触发，覆盖待提交及已提交修改；这不证明所有第三方
UCI 写入器在“最终检查到提交”的任意竞争窗口中具备全局事务隔离。
硬件卸载 option 在基线为 `0`，这里检验软件卸载路径；其他 option 组合继续由原有
单元测试覆盖，不作为虚拟硬件卸载验证。

## 证据

结果保存在 VM 的 `/root/ackrun-openwrt-it.XXXXXX/`：

- `test.log`：每个真实子测试和最终退出结果。
- `environment.txt`：OpenWrt/内核/软件包版本、工具与测试二进制哈希、源码提交。
- `evidence/`：各阶段 UCI export/delta、nft JSON、conntrack、存在时的防护/卸载恢复记录。
- `outer-*.before` / `outer-*.after`：外层 VM 不被修改的检查。

退出 VM 前把该目录与镜像 SHA-256、构建包的源码标识一起复制出来。只有真实运行
返回 0、出现最终 `PASS: real OpenWrt offload integration` 且上述子测试全部 PASS，
才能把对应 OpenWrt/内核/包版本填写为集成回归通过。脚本不发布版本、不安装 IPK。
