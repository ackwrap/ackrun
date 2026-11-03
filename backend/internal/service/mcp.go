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

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

const MCPEndpoint = "/mcp"

type MCPService struct {
	store *store.Store
	mu    sync.Mutex
}

func NewMCPService(db *store.Store) *MCPService { return &MCPService{store: db} }

func (svc *MCPService) Settings() (*model.MCPSettings, error) {
	cfg, err := svc.store.GetMCPConfig()
	if err != nil {
		return nil, err
	}
	return &model.MCPSettings{Enabled: cfg.Enabled, TokenConfigured: cfg.TokenHash != "", EndpointPath: MCPEndpoint}, nil
}

func (svc *MCPService) UpdateSettings(request model.MCPSettingsRequest) (*model.MCPSettings, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	cfg, err := svc.store.GetMCPConfig()
	if err != nil {
		return nil, err
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
	if err := svc.store.SetMCPConfig(cfg); err != nil {
		return nil, err
	}
	logging.Info("mcp.settings", "更新 Ackrun MCP 配置: enabled=%t token_changed=%t", cfg.Enabled, token != "")
	return &model.MCPSettings{Enabled: cfg.Enabled, TokenConfigured: cfg.TokenHash != "", EndpointPath: MCPEndpoint, Token: token}, nil
}

func (svc *MCPService) Authenticate(token string) (enabled, authenticated bool, err error) {
	cfg, err := svc.store.GetMCPConfig()
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
