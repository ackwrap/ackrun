# Ackwrap GUI

`ackwrap-gui.exe` 是 Windows 桌面壳，只负责 Service 探测、单实例窗口和 WebView2 展示。完整 Vue 页面、REST API、WebSocket、数据库和 sing-box 生命周期仍由 `ackwrap-service.exe` 提供。

默认连接地址：

```text
http://127.0.0.1:18080
```

GUI 会先校验 `/api/v1/runtime` 返回 Ackwrap runtime 状态，再导航到 Service 页面。关闭 GUI 不会向 Service 或 sing-box 发送停止命令。

## 开发环境

- Go 1.26.3+
- Wails CLI v2.13.0
- Microsoft Edge WebView2 Runtime

安装固定版本 Wails CLI：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

## 验证

```powershell
go test ./...
go vet ./...
go build ./...
wails build -nopackage -clean
```

开发模式：

```powershell
wails dev
```

在 Windows Service 入口尚未完成前，可以让现有 backend 临时监听 18080：

```powershell
$env:ACKWRAP_LISTEN_ADDR = "127.0.0.1:18080"
Set-Location ..\backend
go run ./cmd/server
```

也可以为本地测试覆盖 GUI 目标地址，但只允许 loopback HTTP URL：

```powershell
$env:ACKWRAP_GUI_SERVICE_URL = "http://127.0.0.1:18081"
wails dev
```

## 源码边界

- `main.go`：Wails 窗口配置、静态 loader 和单实例。
- `app.go`：Service URL 校验、runtime 探测、重试与安全导航。
- `loader/`：Service 不可用时展示的轻量页面，不复制业务 Vue。
- `windows.md`：完整 Windows Service 与 GUI 实施计划。

`build/bin/` 和 Wails 生成的 `loader/wailsjs/` 均为本地生成目录，不得提交。
