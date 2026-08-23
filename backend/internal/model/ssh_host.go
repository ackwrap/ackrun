package model

type SSHCredential struct {
	ID                   int64  `json:"id"`
	Name                 string `json:"name"`
	AuthType             string `json:"auth_type"`
	HasSecret            bool   `json:"has_secret"`
	KeyFingerprint       string `json:"key_fingerprint,omitempty"`
	SecretContext        string `json:"-"`
	SecretCiphertext     []byte `json:"-"`
	SecretNonce          []byte `json:"-"`
	PassphraseCiphertext []byte `json:"-"`
	PassphraseNonce      []byte `json:"-"`
	KeyVersion           int    `json:"-"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`
}

type SSHCredentialRequest struct {
	Name       string `json:"name"`
	AuthType   string `json:"auth_type"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

type SSHHost struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	GroupName        string   `json:"group_name"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Username         string   `json:"username"`
	CredentialID     int64    `json:"credential_id"`
	CredentialName   string   `json:"credential_name"`
	ConnectionMode   string   `json:"connection_mode"`
	NodeExposureID   *int64   `json:"node_exposure_id,omitempty"`
	NodeExposureName string   `json:"node_exposure_name,omitempty"`
	TerminalType     string   `json:"terminal_type"`
	Enabled          bool     `json:"enabled"`
	Tags             []string `json:"tags"`
	Notes            string   `json:"notes"`
	HostKeyStatus    string   `json:"host_key_status"`
	LastStatus       string   `json:"last_status"`
	LastLatencyMS    int64    `json:"last_latency_ms"`
	LastErrorCode    string   `json:"last_error_code"`
	LastErrorMessage string   `json:"last_error_message"`
	LastCheckedAt    int64    `json:"last_checked_at"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
}

type SSHHostRequest struct {
	Name           string   `json:"name"`
	GroupName      string   `json:"group_name"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Username       string   `json:"username"`
	CredentialID   int64    `json:"credential_id"`
	ConnectionMode string   `json:"connection_mode"`
	NodeExposureID *int64   `json:"node_exposure_id"`
	TerminalType   string   `json:"terminal_type"`
	Enabled        bool     `json:"enabled"`
	Tags           []string `json:"tags"`
	Notes          string   `json:"notes"`
}

type SSHHostKey struct {
	HostID            int64  `json:"host_id"`
	KeyType           string `json:"key_type"`
	PublicKey         string `json:"-"`
	FingerprintSHA256 string `json:"fingerprint_sha256"`
	FirstSeenAt       int64  `json:"first_seen_at"`
	LastSeenAt        int64  `json:"last_seen_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

type SSHHostKeyTrustRequest struct {
	ChallengeID       string `json:"challenge_id"`
	FingerprintSHA256 string `json:"fingerprint_sha256"`
}

type SSHHostKeyChallenge struct {
	ChallengeID        string `json:"challenge_id"`
	KeyType            string `json:"key_type"`
	FingerprintSHA256  string `json:"fingerprint_sha256"`
	TrustedFingerprint string `json:"trusted_fingerprint,omitempty"`
	ExpiresAt          int64  `json:"expires_at"`
}

type SSHConnectionTestResult struct {
	Success           bool   `json:"success"`
	LatencyMS         int64  `json:"latency_ms"`
	HandshakeMS       int64  `json:"handshake_ms"`
	ServerVersion     string `json:"server_version"`
	KeyType           string `json:"key_type"`
	FingerprintSHA256 string `json:"fingerprint_sha256"`
	ConnectionMode    string `json:"connection_mode"`
}

type SSHSessionAudit struct {
	ID             int64  `json:"id"`
	SessionIDHash  string `json:"session_id_hash,omitempty"`
	HostID         int64  `json:"host_id"`
	HostName       string `json:"host_name"`
	EventType      string `json:"event_type"`
	ConnectionMode string `json:"connection_mode"`
	NodeExposureID *int64 `json:"node_exposure_id,omitempty"`
	Result         string `json:"result"`
	ErrorCode      string `json:"error_code,omitempty"`
	DurationMS     int64  `json:"duration_ms"`
	CreatedAt      int64  `json:"created_at"`
}

type SSHSessionCreateRequest struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}

type SSHSessionCreateResponse struct {
	SessionID   string `json:"session_id"`
	AttachToken string `json:"attach_token"`
	SFTPToken   string `json:"sftp_token"`
	ExpiresAt   int64  `json:"expires_at"`
}
