package model

type SSHMCPSettings struct {
	Enabled         bool   `json:"enabled"`
	LocalEnabled    bool   `json:"local_enabled"`
	LocalName       string `json:"local_name"`
	TokenConfigured bool   `json:"token_configured"`
	EndpointPath    string `json:"endpoint_path"`
	Token           string `json:"token,omitempty"`
}

type SSHMCPSettingsRequest struct {
	Enabled       bool    `json:"enabled"`
	LocalEnabled  *bool   `json:"local_enabled,omitempty"`
	LocalName     *string `json:"local_name,omitempty"`
	Token         string  `json:"token"`
	GenerateToken bool    `json:"generate_token"`
}

type SSHMCPExecRequest struct {
	HostID  int64  `json:"host_id" jsonschema:"SSH host ID from ssh_list_servers"`
	Command string `json:"command" jsonschema:"Shell command executed on the selected host. Built-in host_id=-1 runs on the AckWrap server when local access is enabled, using sh on Unix or PowerShell on Windows and the AckWrap process account"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"Timeout in seconds; default 30, maximum 600"`
}

type SSHMCPExecResult struct {
	HostID    int64  `json:"host_id"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated"`
}

type SSHMCPFileRequest struct {
	HostID int64  `json:"host_id" jsonschema:"SSH host ID from ssh_list_servers"`
	Path   string `json:"path" jsonschema:"Path on the selected host, including the AckWrap server for built-in local host_id=-1; never a path on the MCP caller's machine"`
}

type SSHMCPWriteRequest struct {
	HostID    int64  `json:"host_id"`
	Path      string `json:"path" jsonschema:"Path on the selected host; local host_id=-1 means the AckWrap server"`
	Content   string `json:"content" jsonschema:"UTF-8 text for ssh_write_file or base64 bytes for ssh_upload; maximum 1 MiB decoded"`
	Overwrite bool   `json:"overwrite,omitempty" jsonschema:"Allow replacing an existing regular file; default false"`
}

type SSHMCPTransferRequest struct {
	HostID    int64  `json:"host_id"`
	Path      string `json:"path" jsonschema:"Destination path on the selected host for upload, or source for download. Built-in local host_id=-1 is the AckWrap server, not the MCP caller. Relative local paths resolve from the service account home directory"`
	Overwrite bool   `json:"overwrite,omitempty" jsonschema:"Explicitly allow replacing an existing regular file"`
	SHA1      string `json:"sha1,omitempty" jsonschema:"Optional expected SHA-1 hex digest (40 hex characters); checked before publishing an upload"`
}

type SSHMCPUploadTicket struct {
	Status           string            `json:"status"`
	UploadURL        string            `json:"upload_url"`
	EndpointPath     string            `json:"endpoint_path"`
	Method           string            `json:"method"`
	Headers          map[string]string `json:"headers"`
	ExpiresAt        int64             `json:"expires_at"`
	ExpiresInSeconds int               `json:"expires_in_seconds"`
	TimeoutSeconds   int               `json:"timeout_seconds"`
	SingleUse        bool              `json:"single_use"`
	Instructions     string            `json:"instructions"`
}

type SSHMCPUploadRequest struct {
	SSHMCPTransferRequest
	SourceURL string  `json:"source_url,omitempty" jsonschema:"Public HTTP(S) download URL to stream directly to the SSH host; private, loopback, link-local, carrier-grade NAT, unspecified, and multicast addresses are rejected; omit to obtain a binary HTTP PUT endpoint for a file on the client's computer"`
	Content   *string `json:"content,omitempty" jsonschema:"Legacy inline base64 for small files only; use source_url or the returned HTTP endpoint for deployment packages"`
}

type SSHMCPDownloadRequest struct {
	HostID int64  `json:"host_id"`
	Path   string `json:"path"`
	Inline bool   `json:"inline,omitempty" jsonschema:"Legacy inline base64 for small files; default false returns a streaming HTTP GET endpoint"`
}

type SSHMCPTransferResult struct {
	Success bool   `json:"success"`
	Bytes   int64  `json:"bytes"`
	SHA1    string `json:"sha1"`
}
