package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

const SSHMCPEndpoint = "/mcp/ssh"

type SSHMCPService struct {
	store   *store.Store
	mu      sync.Mutex
	uploads map[string]sshMCPUploadGrant
	now     func() time.Time
}

func NewSSHMCPService(db *store.Store) *SSHMCPService {
	return &SSHMCPService{store: db, now: time.Now}
}

func (svc *SSHMCPService) Settings() (*model.SSHMCPSettings, error) {
	cfg, err := svc.store.GetSSHMCPConfig()
	if err != nil {
		return nil, err
	}
	return &model.SSHMCPSettings{Enabled: cfg.Enabled, LocalEnabled: cfg.LocalEnabled, LocalName: cfg.LocalName, TokenConfigured: cfg.TokenHash != "", EndpointPath: SSHMCPEndpoint}, nil
}

func (svc *SSHMCPService) UpdateSettings(request model.SSHMCPSettingsRequest) (*model.SSHMCPSettings, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	cfg, err := svc.store.GetSSHMCPConfig()
	if err != nil {
		return nil, err
	}
	if request.LocalName != nil {
		name := strings.TrimSpace(*request.LocalName)
		if utf8.RuneCountInString(name) > 128 || strings.ContainsAny(name, "\x00\r\n") {
			return nil, errors.New("本机名称不能超过 128 个字符或包含换行、NUL")
		}
		if name == "" {
			name = "本机"
		}
		cfg.LocalName = name
	}
	if request.GenerateToken && request.Token != "" {
		return nil, errors.New("自动生成和自定义 Token 不能同时选择")
	}
	token := request.Token
	if request.GenerateToken {
		var random [32]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, errors.New("生成 MCP Token 失败")
		}
		token = base64.RawURLEncoding.EncodeToString(random[:])
	}
	if token != "" {
		if len(token) < 32 || len(token) > 256 || strings.IndexFunc(token, func(r rune) bool { return r < 33 || r > 126 }) >= 0 {
			return nil, errors.New("Token 必须为 32 到 256 位非空白 ASCII 字符")
		}
		hash := sha256.Sum256([]byte(token))
		cfg.TokenHash = hex.EncodeToString(hash[:])
	}
	if request.Enabled && cfg.TokenHash == "" {
		return nil, errors.New("启用 MCP 前请生成或设置 Token")
	}
	cfg.Enabled = request.Enabled
	localDisabled := cfg.LocalEnabled && request.LocalEnabled != nil && !*request.LocalEnabled
	if request.LocalEnabled != nil {
		cfg.LocalEnabled = *request.LocalEnabled
	}
	if err := svc.store.SetSSHMCPConfig(cfg); err != nil {
		return nil, err
	}
	if !cfg.Enabled || token != "" {
		clear(svc.uploads)
	} else if localDisabled {
		for id, grant := range svc.uploads {
			if grant.request.HostID == SSHMCPLocalHostID {
				delete(svc.uploads, id)
			}
		}
	}
	logging.Info("ssh_mcp.settings", "更新 SSH MCP 配置: enabled=%t local_enabled=%t token_changed=%t", cfg.Enabled, cfg.LocalEnabled, token != "")
	return &model.SSHMCPSettings{Enabled: cfg.Enabled, LocalEnabled: cfg.LocalEnabled, LocalName: cfg.LocalName, TokenConfigured: cfg.TokenHash != "", EndpointPath: SSHMCPEndpoint, Token: token}, nil
}

func (svc *SSHMCPService) Authenticate(token string) (enabled, authenticated bool, err error) {
	cfg, err := svc.store.GetSSHMCPConfig()
	if err != nil || !cfg.Enabled {
		return false, false, err
	}
	expected, err := hex.DecodeString(cfg.TokenHash)
	if err != nil || len(expected) != sha256.Size {
		return true, false, nil
	}
	actual := sha256.Sum256([]byte(token))
	return true, token != "" && subtle.ConstantTimeCompare(expected, actual[:]) == 1, nil
}
