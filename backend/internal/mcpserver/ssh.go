package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

type hostInput struct {
	HostID int64 `json:"host_id" jsonschema:"SSH host ID from ssh_list_servers"`
}

func NewSSHServer(svc *service.SSHHostService) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "ackwrap-ssh", Version: "1.0.0"}, &mcp.ServerOptions{
		Instructions: "Operate SSH hosts saved in Ackwrap. Use host_id from ssh_list_servers. Existing credentials and connection paths are reused. Unknown or changed host keys must be confirmed by the user in Ackwrap. File paths always refer to the remote SSH host, never to the MCP client's computer. For large file upload, supply source_url or call ssh_upload to obtain an HTTP PUT endpoint, then use a local HTTP/file tool to stream the file with the same MCP bearer token. For download, call ssh_download and save its HTTP GET response with a local HTTP/file tool. A ready response only prepares a transfer, not completes it. Never encode deployment packages as inline base64.",
	})
	addTool(server, "ssh_list_servers", "列出 Ackwrap SSH 主机，不返回密码或私钥。", func(_ context.Context, _ struct{}) (any, error) {
		return svc.ListHosts()
	})
	addTool(server, "ssh_list_credentials", "列出可用于主机配置的凭据 ID 和元数据，不返回凭据内容。", func(_ context.Context, _ struct{}) (any, error) {
		return svc.ListCredentials()
	})
	addTool(server, "ssh_add_server", "添加主机，credential_id 引用已保存凭据；需要启用时设置 enabled=true。", func(_ context.Context, input model.SSHHostRequest) (any, error) {
		return svc.CreateHost(input)
	})
	addTool(server, "ssh_update_server", "完整更新主机配置，先从 ssh_list_servers 获取现有字段；省略字段会恢复默认值。", func(_ context.Context, input struct {
		HostID int64                `json:"host_id"`
		Config model.SSHHostRequest `json:"config"`
	}) (any, error) {
		return svc.UpdateHost(input.HostID, input.Config)
	})
	addTool(server, "ssh_delete_server", "删除指定 SSH 主机。", func(_ context.Context, input hostInput) (any, error) {
		err := svc.DeleteHost(input.HostID)
		return map[string]any{"success": err == nil}, err
	})
	addTool(server, "ssh_test_connection", "测试已保存主机的 SSH 连通性，未知 Host Key 需要在 Ackwrap 页面确认。", func(ctx context.Context, input hostInput) (any, error) {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		return svc.TestHost(ctx, input.HostID)
	})
	addTool(server, "ssh_exec", "在指定主机执行命令，返回 stdout、stderr 和退出码；非零退出码表示失败。", func(ctx context.Context, input model.SSHMCPExecRequest) (any, error) {
		return svc.ExecuteMCP(ctx, input)
	})
	addTool(server, "ssh_exec_multi", "逐个在最多 10 台主机执行相同命令，单台失败不影响其余主机。", func(ctx context.Context, input struct {
		HostIDs []int64 `json:"host_ids"`
		Command string  `json:"command"`
		Timeout int     `json:"timeout,omitempty"`
	}) (any, error) {
		if len(input.HostIDs) == 0 || len(input.HostIDs) > 10 {
			return nil, errors.New("必须选择 1 到 10 台 SSH 主机")
		}
		results := make([]any, 0, len(input.HostIDs))
		for _, id := range input.HostIDs {
			if err := ctx.Err(); err != nil {
				return results, err
			}
			result, err := svc.ExecuteMCP(ctx, model.SSHMCPExecRequest{HostID: id, Command: input.Command, Timeout: input.Timeout})
			item := map[string]any{"host_id": id, "result": result, "success": err == nil}
			if err != nil {
				item["error"] = toolError(err)
			}
			results = append(results, item)
		}
		return results, nil
	})
	for _, action := range []string{"read_file", "list_dir", "stat"} {
		descriptions := map[string]string{
			"read_file": "读取远程 UTF-8 文本，最大 1 MiB。",
			"list_dir":  "列出远程目录，最多返回 1000 项，更多时标记 truncated。",
			"stat":      "读取远程文件大小、权限和修改时间。",
		}
		addTool(server, "ssh_"+action, descriptions[action], func(ctx context.Context, input model.SSHMCPFileRequest) (any, error) {
			return svc.ReadMCPFile(ctx, input, action)
		})
	}
	addTool(server, "ssh_write_file", "写入远程 UTF-8 文本，最大 1 MiB；覆盖已有文件需要 overwrite=true。", func(ctx context.Context, input model.SSHMCPWriteRequest) (any, error) {
		err := svc.WriteMCPFile(ctx, input, false)
		return map[string]any{"success": err == nil}, err
	})
	addTool(server, "ssh_upload", "流式上传大文件，无总大小限制。提供 source_url 可直接下载并传到 SSH 主机；省略则返回 HTTP PUT 地址，由客户端以原始字节上传本地文件，需使用同一个 MCP Bearer Token。ready 仅表示上传准备就绪。覆盖需 overwrite=true，可传 sha256 校验。content 仅兼容小文件 Base64。", func(ctx context.Context, input model.SSHMCPUploadRequest) (any, error) {
		return svc.UploadMCP(ctx, input)
	})
	addTool(server, "ssh_download", "返回大文件流式 HTTP GET 地址，无总大小限制；客户端需使用同一个 MCP Bearer Token 将响应保存到本地。ready 不表示已下载。仅小文件可用 inline=true 返回 Base64（最大 1 MiB）。", func(ctx context.Context, input model.SSHMCPDownloadRequest) (any, error) {
		if input.Inline {
			return svc.ReadMCPFile(ctx, model.SSHMCPFileRequest{HostID: input.HostID, Path: input.Path}, "download")
		}
		return svc.PrepareMCPTransfer(model.SSHMCPTransferRequest{HostID: input.HostID, Path: input.Path}, false)
	})
	return server
}

func addTool[Input any](server *mcp.Server, name, description string, call func(context.Context, Input) (any, error)) {
	mcp.AddTool(server, &mcp.Tool{Name: name, Description: description}, func(ctx context.Context, _ *mcp.CallToolRequest, input Input) (*mcp.CallToolResult, any, error) {
		value, err := call(ctx, input)
		payload := map[string]any{"data": value}
		if err != nil {
			payload["error"] = toolError(err)
			logging.Error("ssh_mcp.tool", "MCP 工具调用失败: tool=%s", name)
		} else {
			logging.Info("ssh_mcp.tool", "MCP 工具调用完成: tool=%s", name)
		}
		text, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return nil, nil, errors.New("无法编码 MCP 工具结果")
		}
		return &mcp.CallToolResult{IsError: err != nil, Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}, payload, nil
	})
}

func toolError(err error) string {
	var sshErr *service.SSHServiceError
	if errors.As(err, &sshErr) {
		return sshErr.Code + ": " + sshErr.Message
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "MCP 操作已取消或超时"
	}
	return "MCP 操作失败，请检查参数和 Ackwrap 服务状态"
}
