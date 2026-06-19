# Clash (mihomo) vs sing-box 协议字段完整对比

> 来源：
> - sing-box: `sing-box/option/*.go` + `sing-box/docs/*.md`
> - Clash: `mihomo-Alpha/adapter/outbound/*.go`
>
> **状态：已修复所有高优先级问题 ✅ (2026-06-10)**

---

## 修复状态总览

| 协议 | 关键问题 | 状态 |
|------|---------|------|
| **VMess/VLESS** | packet_encoding 优先级、global_padding、authenticated_length | ✅ 已修复 |
| **Shadowsocks** | udp_over_tcp 对象格式 | ✅ 已修复 |
| **Hysteria2** | obfs 对象格式、带宽解析 | ✅ 已修复 |
| **WireGuard** | local_address 格式、类型安全 | ✅ 已修复 |
| **TUIC** | reduce-rtt、disable-sni 映射 | ✅ 已修复 |
| **VLESS** | xhttp-opts 支持 | ✅ 已修复 |
| **所有 TLS** | client-fingerprint 优先级、隐式 TLS | ✅ 已修复 |
| **通用** | boolOrString int 支持、端口 0 校验、plugin_opts 排序 | ✅ 已修复 |

---

## 1. Shadowsocks

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| Name | string | `name` |
| Server | string | `server` |
| Port | int | `port` |
| Password | string | `password` |
| Cipher | string | `cipher` |
| UDP | bool | `udp` |
| Plugin | string | `plugin` |
| PluginOpts | map[string]any | `plugin-opts` |
| UDPOverTCP | bool | `udp-over-tcp` |
| UDPOverTCPVersion | int | `udp-over-tcp-version` |
| ClientFingerprint | string | `client-fingerprint` |

### sing-box 字段
| 字段 | 类型 | JSON tag |
|------|------|----------|
| Server | string | `server` |
| ServerPort | uint16 | `server_port` |
| Method | string | `method` |
| Password | string | `password` |
| Plugin | string | `plugin` |
| PluginOptions | string | `plugin_opts` |
| Network | NetworkList | `network` |
| UDPOverTCP | *UDPOverTCPOptions | `udp_over_tcp` |
| Multiplex | *OutboundMultiplexOptions | `multiplex` |

### sing-box UDPOverTCPOptions 结构
```go
type _UDPOverTCPOptions struct {
    Enabled bool   `json:"enabled,omitempty"`
    Version uint8  `json:"version,omitempty"`
}
```
MarshalJSON 简化：version=0 时只输出 `true`/`false`，否则输出完整对象。

### 转换问题清单

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ⚠️ | `cipher` → `method` 已处理 | ✅ | line 90-91 |
| ❌ | `plugin-opts` map→string 已修复 | ✅ | 现在 map 转 key=value 字符串 |
| ❌ | `udp-over-tcp` 未映射 | **高** | Clash `udp-over-tcp:true` → sing-box `udp_over_tcp:true` |
| ❌ | `udp-over-tcp-version` 未映射 | 低 | Clash version → sing-box `udp_over_tcp.version` |
| ❓ | `client-fingerprint` 未映射 | 中 | SS 没有 TLS，sing-box SS 不支持 utls |

---

## 2. VMess

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| Name | string | `name` |
| Server | string | `server` |
| Port | int | `port` |
| UUID | string | `uuid` |
| AlterID | int | `alterId` |
| Cipher | string | `cipher` |
| UDP | bool | `udp` |
| Network | string | `network` |
| TLS | bool | `tls` |
| ALPN | []string | `alpn` |
| SkipCertVerify | bool | `skip-cert-verify` |
| Fingerprint | string | `fingerprint` |
| ServerName | string | `servername` |
| PacketAddr | bool | `packet-addr` |
| XUDP | bool | `xudp` |
| PacketEncoding | string | `packet-encoding` |
| GlobalPadding | bool | `global-padding` |
| AuthenticatedLength | bool | `authenticated-length` |
| ClientFingerprint | string | `client-fingerprint` |
| PrivateKey | string | `private-key` |
| RealityOpts | RealityOptions | `reality-opts` |
| ECHOpts | ECHOptions | `ech-opts` |
| HTTPOpts | HTTPOptions | `http-opts` |
| HTTP2Opts | HTTP2Options | `h2-opts` |
| GrpcOpts | GrpcOptions | `grpc-opts` |
| WSOpts | WSOptions | `ws-opts` |

### sing-box 字段
```go
type VMessOutboundOptions struct {
    DialerOptions
    ServerOptions                    // server, server_port
    UUID                string      // `json:"uuid"`
    Security            string      // `json:"security"`
    AlterId             int         // `json:"alter_id"`
    GlobalPadding       bool        // `json:"global_padding"`
    AuthenticatedLength bool        // `json:"authenticated_length"`
    Network             NetworkList // `json:"network"`
    OutboundTLSOptionsContainer     // tls: {...}
    PacketEncoding      string      // `json:"packet_encoding"`
    Multiplex           *OutboundMultiplexOptions
    Transport           *V2RayTransportOptions // transport: {type, path...}
}
```

### 转换问题清单

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ✅ | `alterId` → `alter_id` | ✅ | line 82-84 |
| ✅ | `cipher` → `security` | ✅ | line 88-89 |
| ⚠️ | `Network` (ws/grpc) → `transport.type` | ✅ | line 156-191 |
| ❌ | `global-padding` 未映射 | **高** | 直接复制 `global_padding` |
| ❌ | `authenticated-length` 未映射 | **高** | 直接复制 `authenticated_length` |
| ❌ | `packet-addr:true` 未映射 | **高** | 应设为 `packet_encoding: "packetaddr"` |
| ❌ | `xudp:true` 未映射 | **高** | 应设为 `packet_encoding: "xudp"` |
| ❌ | `packet-encoding` 未映射 | 中 | 若有值应直接复制 `packet_encoding` |
| ⚠️ | `client-fingerprint` → `tls.utls.fingerprint` | **高** | VMess 当前已处理，但需确认 |
| ❌ | `fingerprint` → `tls.utls.fingerprint` | **中** | 和 `client-fingerprint` 重复？ |

---

## 3. VLESS

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| UUID | string | `uuid` |
| Flow | string | `flow` |
| UDP | bool | `udp` |
| PacketAddr | bool | `packet-addr` |
| XUDP | bool | `xudp` |
| PacketEncoding | string | `packet-encoding` |
| Encryption | string | `encryption` |
| Network | string | `network` |
| XHTTPOpts | XHTTPOptions | `xhttp-opts` |
| WSHeaders | map[string]string | `ws-headers` |
| (其余同 VMess) | | |

### sing-box 字段
```go
type VLESSOutboundOptions struct {
    DialerOptions
    ServerOptions
    UUID    string      // `json:"uuid"`
    Flow    string      // `json:"flow"`
    Network NetworkList // `json:"network"`
    OutboundTLSOptionsContainer
    Multiplex      *OutboundMultiplexOptions
    Transport      *V2RayTransportOptions
    PacketEncoding *string // `json:"packet_encoding"`
}
```

### 转换问题清单

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ✅ | `flow` → `flow` | ✅ | line 99 |
| ❌ | `encryption` 未映射 | 低 | outbound 不需要，sing-box vless 默认为 none |
| ❌ | `packet-addr/xudp` 未映射 | **高** | 应设为 `packet_encoding` |
| ❌ | `packet-encoding` 未映射 | 中 | 若有值应直接复制 |
| ❌ | `xhttp-opts` 未映射 | **高** | 应映射为 `transport.type: httpupgrade` |
| ❌ | `ws-headers` 未映射 | 中 | WSHeaders → transport.headers |

---

## 4. Trojan

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| Password | string | `password` |
| UDP | bool | `udp` |
| Network | string | `network` |
| SkipCertVerify | bool | `skip-cert-verify` |
| Fingerprint | string | `fingerprint` |
| ClientFingerprint | string | `client-fingerprint` |
| SNI | string | `sni` |
| GrpcOpts | GrpcOptions | `grpc-opts` |
| WSOpts | WSOptions | `ws-opts` |
| SSOpts | TrojanSSOption | `ss-opts` |
| (其余同 VMess TLS) | | |

### sing-box 字段
```go
type TrojanOutboundOptions struct {
    DialerOptions
    ServerOptions
    Password string      // `json:"password"`
    Network  NetworkList // `json:"network"`
    OutboundTLSOptionsContainer
    Multiplex *OutboundMultiplexOptions
    Transport *V2RayTransportOptions
}
```

### 转换问题清单

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ✅ | 基本字段都正确 | ✅ | password, tls, transport |
| ❓ | `ss-opts` 未映射 | 低 | Clash 特有的 Trojan over SS |
| ❌ | `sni` → `tls.server_name` | 中 | 目前是否已处理？ |

---

## 5. Hysteria2

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| Server | string | `server` |
| Port | int | `port` |
| Ports | string | `ports` |
| HopInterval | string | `hop-interval` |
| Up | string | `up` |
| Down | string | `down` |
| Password | string | `password` |
| Obfs | string | `obfs` |
| ObfsPassword | string | `obfs-password` |
| ObfsMinPacketSize | int | `obfs-min-packet-size` |
| ObfsMaxPacketSize | int | `obfs-max-packet-size` |
| SNI | string | `sni` |
| SkipCertVerify | bool | `skip-cert-verify` |
| Fingerprint | string | `fingerprint` |

### sing-box 字段
```go
type Hysteria2OutboundOptions struct {
    DialerOptions
    ServerOptions
    ServerPorts badoption.Listable[string] // `json:"server_ports"`
    HopInterval badoption.Duration         // `json:"hop_interval"`
    UpMbps      int                        // `json:"up_mbps"`
    DownMbps    int                        // `json:"down_mbps"`
    Obfs        *Hysteria2Obfs             // `json:"obfs"`
    Password    string                     // `json:"password"`
    Network     NetworkList                // `json:"network"`
    OutboundTLSOptionsContainer
    BrutalDebug bool `json:"brutal_debug"`
}
```
```go
type Hysteria2Obfs struct {
    Type     string `json:"type,omitempty"`     // "salamander"
    Password string `json:"password,omitempty"`
}
```

### 转换问题清单

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ❌ | `obfs` + `obfs-password` → `objs:{type,password}` | **高** | 需合并为嵌套对象 |
| ❌ | `ports` → `server_ports` | 中 | 直接复制 |
| ❌ | `hop-interval` → `hop_interval` | 中 | 直接复制 |
| ❌ | `up`(string) → `up_mbps`(int) | **高** | 字符串 "100 Mbps" 需解析为 int |
| ❌ | `down` → `down_mbps` | **高** | 同上 |
| ❌ | `obfs-min-packet-size` / `obfs-max-packet-size` | 低 | sing-box 不支持 |

---

## 6. Hysteria1

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| Protocol | string | `protocol` |
| ObfsProtocol | string | `obfs-protocol` |
| Up | string | `up` |
| UpSpeed | int | `up-speed` |
| Down | string | `down` |
| DownSpeed | int | `down-speed` |
| Auth | string | `auth` |
| AuthString | string | `auth-str` |
| Obfs | string | `obfs` |
| ReceiveWindowConn | int | `recv-window-conn` |
| ReceiveWindow | int | `recv-window` |
| DisableMTUDiscovery | bool | `disable-mtu-discovery` |
| FastOpen | bool | `fast-open` |
| (其余 TLS) | | |

### sing-box 字段
```go
type HysteriaOutboundOptions struct {
    DialerOptions
    ServerOptions
    ServerPorts         badoption.Listable[string]
    HopInterval         badoption.Duration
    Up                  *byteformats.NetworkBytesCompat // `json:"up"`
    UpMbps              int                             // `json:"up_mbps"`
    Down                *byteformats.NetworkBytesCompat // `json:"down"`
    DownMbps            int                             // `json:"down_mbps"`
    Obfs                string                          // `json:"obfs"`
    Auth                []byte                          // `json:"auth"`
    AuthString          string                          // `json:"auth_str"`
    ReceiveWindowConn   uint64                          // `json:"recv_window_conn"`
    ReceiveWindow       uint64                          // `json:"recv_window"`
    DisableMTUDiscovery bool                            // `json:"disable_mtu_discovery"`
    Network             NetworkList
    OutboundTLSOptionsContainer
}
```

### 转换问题清单
| # | 问题 | 严重度 |
|---|------|--------|
| ❌ | `protocol` → 不适用 | 中 - Hysteria1 有 udp/tcp/fake_tcp 等 |
| ❌ | `up-speed`(int) / `down-speed`(int) | 低 |
| ❌ | `fast-open` → `tcp_fast_open` | 低 |

---

## 7. TUIC

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| Token | string | `token` |
| UUID | string | `uuid` |
| Password | string | `password` |
| Ip | string | `ip` |
| HeartbeatInterval | int | `heartbeat-interval` |
| ALPN | []string | `alpn` |
| ReduceRtt | bool | `reduce-rtt` |
| RequestTimeout | int | `request-timeout` |
| UdpRelayMode | string | `udp-relay-mode` |
| CongestionController | string | `congestion-controller` |
| DisableSni | bool | `disable-sni` |
| FastOpen | bool | `fast-open` |

### sing-box 字段
```go
type TUICOutboundOptions struct {
    DialerOptions
    ServerOptions
    UUID              string   // `json:"uuid"`
    Password          string   // `json:"password"`
    CongestionControl string   // `json:"congestion_control"`
    UDPRelayMode      string   // `json:"udp_relay_mode"`
    UDPOverStream     bool     // `json:"udp_over_stream"`
    ZeroRTTHandshake  bool     // `json:"zero_rtt_handshake"`
    Heartbeat         Duration // `json:"heartbeat"`
    Network           NetworkList
    OutboundTLSOptionsContainer
}
```

### 转换问题清单

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ✅ | `congestion-controller` → `congestion_control` | ✅ | |
| ✅ | `udp-relay-mode` → `udp_relay_mode` | ✅ | |
| ❌ | `token` → 不适用 | 低 | sing-box 用 uuid 或 password |
| ❌ | `reduce-rtt` → `zero_rtt_handshake` | 中 | 不同名 |
| ❌ | `disable-sni` → `tls.disable_sni` | 中 | |
| ❌ | `heartbeat-interval` → `heartbeat` (Duration) | 中 | 类型不同 |
| ❌ | `ip` (server IP) | 低 | |
| ❌ | `fast-open` → `tcp_fast_open` | 低 | 在 DialerOptions |

---

## 8. WireGuard

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| PrivateKey | string | `private-key` |
| Workers | int | `workers` |
| MTU | int | `mtu` |
| UDP | bool | `udp` |
| Ip | string | `ip` |
| Ipv6 | string | `ipv6` |
| PublicKey | string | `public-key` |
| PreSharedKey | string | `pre-shared-key` |
| Reserved | []uint8 | `reserved` |
| AllowedIPs | []string | `allowed-ips` |
| PersistentKeepalive | int | `persistent-keepalive` |

### sing-box 字段
```go
type WireGuardEndpointOptions struct {
    System     bool            // `json:"system"`
    Name       string          // `json:"name"`
    MTU        uint32          // `json:"mtu"`
    Address    Listable[Prefix]// `json:"address"`
    PrivateKey string          // `json:"private_key"`
    ListenPort uint16          // `json:"listen_port"`
    Peers      []WireGuardPeer // `json:"peers"`
    UDPTimeout Duration        // `json:"udp_timeout"`
    Workers    int             // `json:"workers"`
    DialerOptions
}

type WireGuardPeer struct {
    Address                     string           // `json:"address"`
    Port                        uint16           // `json:"port"`
    PublicKey                   string           // `json:"public_key"`
    PreSharedKey                string           // `json:"pre_shared_key"`
    AllowedIPs                  Listable[Prefix] // `json:"allowed_ips"`
    PersistentKeepaliveInterval uint16           // `json:"persistent_keepalive_interval"`
    Reserved                    []uint8          // `json:"reserved"`
}
```

### 转换问题清单

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ⚠️ | `ip`/`ipv6` → `address` (netip.Prefix) | **高** | 格式不同 |
| ❌ | `persistent-keepalive` → `persistent_keepalive_interval` | 中 | peer 级别 |
| ❌ | `udp` → 无关 | 低 | WireGuard 本来就是 UDP |

---

## 9. SOCKS

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| Username | string | `username` |
| Password | string | `password` |
| TLS | bool | `tls` |
| UDP | bool | `udp` |
| SkipCertVerify | bool | `skip-cert-verify` |
| Fingerprint | string | `fingerprint` |

### sing-box 字段
```go
type SOCKSOutboundOptions struct {
    DialerOptions
    ServerOptions
    Version    string             // `json:"version"`
    Username   string             // `json:"username"`
    Password   string             // `json:"password"`
    Network    NetworkList        // `json:"network"`
    UDPOverTCP *UDPOverTCPOptions // `json:"udp_over_tcp"`
}
```

### 转换问题清单
| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ❌ | `tls:true` → sing-box 没有 TLS 支持 | **高** | SOCKS outbound 不支持 TLS |
| ❌ | `udp` → `network` | 中 | 类似 ss |

---

## 10. HTTP

### Clash (mihomo) 字段
| 字段 | 类型 | Clash tag |
|------|------|-----------|
| Username | string | `username` |
| Password | string | `password` |
| TLS | bool | `tls` |
| SNI | string | `sni` |
| SkipCertVerify | bool | `skip-cert-verify` |
| Fingerprint | string | `fingerprint` |

### sing-box 字段
```go
type HTTPOutboundOptions struct {
    DialerOptions
    ServerOptions
    Username string   // `json:"username"`
    Password string   // `json:"password"`
    OutboundTLSOptionsContainer  // tls: {enabled...}
    Path    string               // `json:"path"`
    Headers HTTPHeader           // `json:"headers"`
}
```

### 转换问题清单
| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ✅ | TLS 支持 | ✅ | sing-box HTTP 支持 TLS |

---

## 当前 `normalizeClashProxy` 已处理的字段 ✅

从 `clash.go` 当前代码，已正确处理：
- `port` → `server_port` (line 77)
- `uuid` (line 81)
- `alterId` → `alter_id` (line 82-84)
- `cipher` → `security`(vmess) / `method`(ss) (line 86-95)
- `password`, `flow`, `plugin`, `protocol`, `obfs` (line 97-102)
- `obfs-param` → `obfs_param` (line 103-105)
- `auth_str` (line 106)
- `private-key` → `private_key` (line 107-109)
- `public-key` → `public_key` (line 110-112)
- `preshared-key` → `pre_shared_key` (line 113-115)
- `reserved` (line 116)
- `local-address` → `local_address` (line 117-119)
- `mtu`, `username` (line 120-121)
- TLS 嵌套 `{enabled, server_name, insecure, utls, alpn, reality}` (line 123-153)
- Transport 嵌套 `{type, path, headers, service_name, host}` (line 156-191)
- `plugin-opts` map→string (line 194-205)
- `udp` → `network` 当 `udp:false` 时 (line 207-210)

---

## 需要立刻修复的高优先级问题 🚨

| # | 协议 | 问题 | 修复方式 |
|---|------|------|---------|
| 1 | **VMess/VLESS** | `packet-addr`/`xudp` → `packet_encoding` | 添加转换逻辑 |
| 2 | **VMess** | `global-padding`, `authenticated-length` | 直接复制 |
| 3 | **VMess** | `packet-encoding` 直接值 | 直接复制 |
| 4 | **VLESS** | `xhttp-opts` → transport type httpupgrade | 添加 httpupgrade 支持 |
| 5 | **Hysteria2** | `obfs` + `obfs-password` → `obfs:{type,password}` | 合并为嵌套对象 |
| 6 | **Hysteria2** | `up`/`down`(string) → `up_mbps`/`down_mbps`(int) | 解析字符串 |
| 7 | **Shadowsocks** | `udp-over-tcp`/`udp-over-tcp-version` → `udp_over_tcp` | 构建选项对象 |
| 8 | **TUIC** | `reduce-rtt` → `zero_rtt_handshake` | 映射改名 |
| 9 | **TUIC** | `disable-sni` → `tls.disable_sni` | 映射到 TLS |
| 10 | **WireGuard** | `ip`/`ipv6` → `address` | 格式转换 |

---

## 修复记录 (2026-06-10)

所有高优先级问题已在 `backend/internal/parser/clash.go` 中修复完成。

### 已修复列表

| # | 问题 | 代码位置 | 验证状态 |
|---|------|---------|---------|
| 1 | VMess/VLESS `packet_encoding` 优先级 | line 153-160 | ✅ 已编译通过 |
| 2 | VMess `global_padding`, `authenticated_length` | line 144-150 | ✅ 已编译通过 |
| 3 | Shadowsocks `udp_over_tcp` 对象格式 | line 99-113 | ✅ 已编译通过 |
| 4 | Hysteria2 `obfs` 对象格式 + 带宽解析 | line 164-185 | ✅ 已编译通过 |
| 5 | WireGuard `local_address` 格式 + 类型安全 | line 187-217 | ✅ 已编译通过 |
| 6 | TUIC 字段映射 | line 219-233 | ✅ 已编译通过 |
| 7 | TLS fingerprint 优先级 | line 259 | ✅ 已编译通过 |
| 8 | VLESS xhttp-opts → httpupgrade | line 315-330 | ✅ 已编译通过 |
| 9 | `boolOrString` int/float64 支持 | line 380-390 | ✅ 已编译通过 |
| 10 | h2/Reality/Trojan 隐式 TLS | line 235-278 | ✅ 已编译通过 |
| 11 | 端口 0 节点校验 | line 59 | ✅ 已编译通过 |
| 12 | plugin_opts map 排序 + 嵌套值 | line 332-356 | ✅ 已编译通过 |
| 13 | `parseBandwidth` Gbps/Kbps 支持 | line 393-414 | ✅ 已编译通过 |
| 14 | 死代码清理 (`clashToSingboxKeyMap`) | 已删除 | ✅ 已编译通过 |

### 修复细节

**packet_encoding 优先级逻辑：**
```go
if pe, ok := proxy["packet-encoding"]; ok && getString(proxy, "packet-encoding") != "" {
    result["packet_encoding"] = pe
} else if pa, ok := proxy["packet-addr"]; ok && boolOrString(pa) {
    result["packet_encoding"] = "packetaddr"
} else if xudp, ok := proxy["xudp"]; ok && boolOrString(xudp) {
    result["packet_encoding"] = "xudp"
}
```

**WireGuard local_address 优先级：**
```go
// 优先使用 local-address，没有时才用 ip/ipv6
// 确保输出为字符串数组，支持 CIDR 自动补全
```

**parseBandwidth 单位支持：**
- `"1 Gbps"` → `1000`
- `"100 Mbps"` → `100`
- `"1000 Kbps"` → `1` (kbps < 1000 直接舍弃)

**TLS 隐式启用逻辑：**
- h2/http 传输自动启用 TLS
- Reality 配置自动启用 TLS
- Trojan 有 TLS 选项时生成 TLS 配置

### 验证方式

```bash
cd backend
go build ./...
go test ./...
go vet ./...
```

全部通过 ✅
