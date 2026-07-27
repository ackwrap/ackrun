package service

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
)

const (
	singboxRuntimeAPIHost    = "127.0.0.1"
	singboxRuntimeAPIPort    = 9097
	singboxRuntimeAPIBaseURL = "http://127.0.0.1:9097/api/v1"
	coreAPITokenBytes        = 32
	coreAPIResponseBodyLimit = 1 << 20
)

type nodeExposureRuntime interface {
	Sync(items []model.NodeExposureWithNode) error
	Upsert(item model.NodeExposureWithNode) error
	Delete(id int64) error
}

type NodeExposureRuntimeClient struct {
	baseURL    string
	secret     string
	httpClient *http.Client
}

type coreNodeExposureRequest struct {
	Inbound     map[string]interface{} `json:"inbound"`
	OutboundTag string                 `json:"outbound_tag"`
}

type coreNodeExposureList struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
}

type RuntimeRoutingConfig struct {
	Routes                  []RuntimeRoute `json:"routes"`
	Leases                  []RuntimeLease `json:"leases"`
	UnhealthyOutbounds      []string       `json:"unhealthy_outbounds"`
	AccessEventsEnabled     bool           `json:"access_events_enabled"`
	AccessEventsPrivacyMode string         `json:"access_events_privacy_mode"`
}

type RuntimeRoute struct {
	ID                  string   `json:"id"`
	Priority            int      `json:"priority"`
	Platform            string   `json:"platform"`
	InboundTags         []string `json:"inbound_tags"`
	SourcePrefixes      []string `json:"source_prefixes"`
	Domains             []string `json:"domains"`
	DomainSuffixes      []string `json:"domain_suffixes"`
	DomainKeywords      []string `json:"domain_keywords"`
	DestinationPrefixes []string `json:"destination_prefixes"`
	OutboundTag         string   `json:"outbound_tag"`
	FallbackOutboundTag string   `json:"fallback_outbound_tag"`
}

type RuntimeLease struct {
	ID                  string   `json:"id"`
	SourcePrefix        string   `json:"source_prefix"`
	InboundTags         []string `json:"inbound_tags"`
	Platform            string   `json:"platform"`
	OutboundTag         string   `json:"outbound_tag"`
	FallbackOutboundTag string   `json:"fallback_outbound_tag"`
	ExpiresAt           int64    `json:"expires_at"`
}

type RuntimeAccessEvent struct {
	ID            uint64 `json:"id"`
	Time          int64  `json:"time"`
	Network       string `json:"network"`
	Inbound       string `json:"inbound"`
	SourceIP      string `json:"source_ip"`
	DestinationIP string `json:"destination_ip"`
	Domain        string `json:"domain"`
	OutboundTag   string `json:"outbound_tag"`
	Platform      string `json:"platform"`
	RouteID       string `json:"route_id"`
	LeaseID       string `json:"lease_id"`
	Decision      string `json:"decision"`
	Error         string `json:"error"`
}

type RuntimeAccessEventList struct {
	Items    []RuntimeAccessEvent `json:"items"`
	LatestID uint64               `json:"latest_id"`
}

type coreAPIErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func LoadOrCreateCoreAPIToken(runtimePaths *paths.Paths) (string, error) {
	path := runtimePaths.CoreAPITokenPath()
	if token, err := readCoreAPIToken(path); err == nil {
		if chmodErr := os.Chmod(path, 0o600); chmodErr != nil {
			return "", fmt.Errorf("protect core API token: %w", chmodErr)
		}
		return token, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}

	random := make([]byte, coreAPITokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate core API token: %w", err)
	}
	token := hex.EncodeToString(random)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return readCoreAPIToken(path)
	}
	if err != nil {
		return "", fmt.Errorf("create core API token: %w", err)
	}
	written := false
	defer func() {
		_ = file.Close()
		if !written {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.WriteString(token + "\n"); err != nil {
		return "", fmt.Errorf("write core API token: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync core API token: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close core API token: %w", err)
	}
	written = true
	return token, nil
}

func readCoreAPIToken(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(content))
	decoded, decodeErr := hex.DecodeString(token)
	if decodeErr != nil || len(decoded) != coreAPITokenBytes {
		return "", errors.New("core API token file is invalid")
	}
	return token, nil
}

func NewNodeExposureRuntimeClient(secret string) *NodeExposureRuntimeClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &NodeExposureRuntimeClient{
		baseURL: singboxRuntimeAPIBaseURL,
		secret:  secret,
		httpClient: &http.Client{
			Timeout:   5 * time.Second,
			Transport: transport,
		},
	}
}

func singboxRuntimeAPIServiceConfig(secret string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "api",
		"tag":         "ackwrap-runtime-api",
		"listen":      singboxRuntimeAPIHost,
		"listen_port": singboxRuntimeAPIPort,
		"secret":      secret,
	}
}

func (c *NodeExposureRuntimeClient) Sync(items []model.NodeExposureWithNode) error {
	if err := c.waitHealthy(5 * time.Second); err != nil {
		return fmt.Errorf("等待核心运行时 API 就绪失败: %w", err)
	}
	activeIDs, err := c.listIDs()
	if err != nil {
		return err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	desiredIDs := make(map[int64]bool)
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		if !item.NodeExists || !item.NodeEnabled {
			return fmt.Errorf("节点暴露 %q 的目标节点不存在或已停用", item.Name)
		}
		if err := c.Upsert(item); err != nil {
			return fmt.Errorf("同步节点暴露 %d 失败: %w", item.ID, err)
		}
		desiredIDs[item.ID] = true
	}
	for id := range activeIDs {
		if desiredIDs[id] {
			continue
		}
		if err := c.Delete(id); err != nil {
			return fmt.Errorf("清理节点暴露 %d 失败: %w", id, err)
		}
	}
	return nil
}

func (c *NodeExposureRuntimeClient) Upsert(item model.NodeExposureWithNode) error {
	if !item.Enabled {
		return c.Delete(item.ID)
	}
	payload := coreNodeExposureRequest{
		Inbound: map[string]interface{}{
			"type":        item.InboundType,
			"listen":      item.Listen,
			"listen_port": item.ListenPort,
		},
		OutboundTag: nodeExposureOutboundTag(item),
	}
	if payload.OutboundTag == "" {
		return errors.New("目标节点无法生成出站标签")
	}
	if item.Username != "" && item.Password != "" {
		payload.Inbound["users"] = []map[string]string{{
			"username": item.Username,
			"password": item.Password,
		}}
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("编码节点暴露请求: %w", err)
	}
	_, err = c.request(http.MethodPut, "/node-exposures/"+strconv.FormatInt(item.ID, 10), content, http.StatusOK)
	return err
}

func (c *NodeExposureRuntimeClient) Delete(id int64) error {
	_, err := c.request(
		http.MethodDelete,
		"/node-exposures/"+strconv.FormatInt(id, 10),
		nil,
		http.StatusNoContent,
		http.StatusNotFound,
	)
	return err
}

func (c *NodeExposureRuntimeClient) GetRuntimeRouting() (RuntimeRoutingConfig, error) {
	content, err := c.request(http.MethodGet, "/runtime-routing", nil, http.StatusOK)
	if err != nil {
		return RuntimeRoutingConfig{}, err
	}
	var result RuntimeRoutingConfig
	if err := json.Unmarshal(content, &result); err != nil {
		return RuntimeRoutingConfig{}, fmt.Errorf("解析核心运行时路由响应: %w", err)
	}
	return result, nil
}

func (c *NodeExposureRuntimeClient) PutRuntimeRouting(config RuntimeRoutingConfig) error {
	content, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("编码核心运行时路由请求: %w", err)
	}
	_, err = c.request(http.MethodPut, "/runtime-routing", content, http.StatusOK)
	return err
}

func (c *NodeExposureRuntimeClient) GetAccessEvents(after uint64, limit int) (RuntimeAccessEventList, error) {
	if limit < 1 || limit > 500 {
		return RuntimeAccessEventList{}, fmt.Errorf("核心访问事件 limit 必须在 1 到 500 之间")
	}
	path := fmt.Sprintf("/access-events?after=%d&limit=%d", after, limit)
	content, err := c.request(http.MethodGet, path, nil, http.StatusOK)
	if err != nil {
		return RuntimeAccessEventList{}, err
	}
	var result RuntimeAccessEventList
	if err := json.Unmarshal(content, &result); err != nil {
		return RuntimeAccessEventList{}, fmt.Errorf("解析核心访问事件响应: %w", err)
	}
	return result, nil
}

func (c *NodeExposureRuntimeClient) waitHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		_, lastErr = c.request(http.MethodGet, "/health", nil, http.StatusOK)
		if lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (c *NodeExposureRuntimeClient) listIDs() (map[int64]bool, error) {
	content, err := c.request(http.MethodGet, "/node-exposures", nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var response coreNodeExposureList
	if err := json.Unmarshal(content, &response); err != nil {
		return nil, fmt.Errorf("解析核心节点暴露列表: %w", err)
	}
	result := make(map[int64]bool, len(response.Items))
	for _, item := range response.Items {
		id, err := strconv.ParseInt(item.ID, 10, 64)
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("核心返回无效的节点暴露 ID: %q", item.ID)
		}
		result[id] = true
	}
	return result, nil
}

func (c *NodeExposureRuntimeClient) request(method, path string, content []byte, expectedStatuses ...int) ([]byte, error) {
	request, err := http.NewRequest(method, c.baseURL+path, bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.secret)
	if content != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求核心运行时 API: %w", err)
	}
	defer response.Body.Close()
	responseContent, readErr := io.ReadAll(io.LimitReader(response.Body, coreAPIResponseBodyLimit+1))
	if readErr != nil {
		return nil, fmt.Errorf("读取核心运行时 API 响应: %w", readErr)
	}
	if len(responseContent) > coreAPIResponseBodyLimit {
		return nil, fmt.Errorf("核心运行时 API 响应超过 %d 字节限制", coreAPIResponseBodyLimit)
	}
	for _, status := range expectedStatuses {
		if response.StatusCode == status {
			return responseContent, nil
		}
	}
	message := strings.TrimSpace(string(responseContent))
	var envelope coreAPIErrorEnvelope
	if json.Unmarshal(responseContent, &envelope) == nil && envelope.Error.Message != "" {
		message = envelope.Error.Message
	}
	message = strings.ReplaceAll(message, c.secret, "[REDACTED]")
	if message == "" {
		message = response.Status
	}
	return nil, fmt.Errorf("核心运行时 API 返回 %s: %s", response.Status, message)
}

func nodeExposureOutboundTag(item model.NodeExposureWithNode) string {
	base := sanitizeOutboundTag(item.NodeName)
	if base == "" {
		base = sanitizeOutboundTag(item.NodeType)
	}
	if base == "" {
		base = "node"
	}
	return fmt.Sprintf("%s-%s", base, item.NodeUID)
}
