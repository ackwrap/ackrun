package model

type SSHMCPSettings struct {
	Enabled         bool   `json:"enabled"`
	TokenConfigured bool   `json:"token_configured"`
	EndpointPath    string `json:"endpoint_path"`
	Token           string `json:"token,omitempty"`
}

type SSHMCPSettingsRequest struct {
	Enabled       bool   `json:"enabled"`
	Token         string `json:"token"`
	GenerateToken bool   `json:"generate_token"`
}

type SSHMCPExecRequest struct {
	HostID  int64  `json:"host_id" jsonschema:"SSH host ID from ssh_list_servers"`
	Command string `json:"command" jsonschema:"Shell command executed on the remote SSH host"`
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
	Path   string `json:"path" jsonschema:"Remote path on the SSH host"`
}

type SSHMCPWriteRequest struct {
	HostID    int64  `json:"host_id"`
	Path      string `json:"path" jsonschema:"Remote path on the SSH host"`
	Content   string `json:"content" jsonschema:"UTF-8 text for ssh_write_file or base64 bytes for ssh_upload; maximum 1 MiB decoded"`
	Overwrite bool   `json:"overwrite,omitempty" jsonschema:"Allow replacing an existing regular file; default false"`
}
