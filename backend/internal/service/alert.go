package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

var (
	ErrAlertInvalid              = errors.New("告警通知配置无效")
	ErrAlertNotFound             = errors.New("告警通知资源不存在")
	ErrAlertConflict             = errors.New("告警通知资源冲突")
	ErrAlertDelivery             = errors.New("告警通知投递失败")
	ErrAlertSecretKeyUnavailable = errors.New("alert secret key is unavailable")
)

var (
	telegramTokenPattern = regexp.MustCompile(`^[0-9]{5,20}:[A-Za-z0-9_-]{20,}$`)
	alertURLPattern      = regexp.MustCompile(`(?i)https?://[^\s]+`)
	alertSecretPattern   = regexp.MustCompile(`(?i)(token|password|secret|uuid|private_key|authorization)\s*[:=]\s*[^\s]+`)
)

type AlertService struct {
	store  *store.Store
	cipher *secretCipher
	now    func() time.Time

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
	closed bool
	events chan model.AlertEvent

	httpClient       alertHTTPClient
	publicHTTPClient alertHTTPClient
	telegramBaseURL  string
}

func NewAlertService(db *store.Store, p *paths.Paths) (*AlertService, error) {
	channels, err := db.ListAlertChannels()
	if err != nil {
		return nil, fmt.Errorf("inspect alert channels: %w", err)
	}
	hasEncryptedSecrets := false
	for _, channel := range channels {
		hasEncryptedSecrets = hasEncryptedSecrets || channel.HasSecret
	}
	cipher, err := loadOrCreateSecretCipher(p.AlertSecretKeyPath(), !hasEncryptedSecrets, ErrAlertSecretKeyUnavailable)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	svc := &AlertService{
		store: db, cipher: cipher, now: time.Now, ctx: ctx, cancel: cancel,
		httpClient: newAlertHTTPClient(), telegramBaseURL: "https://api.telegram.org",
		publicHTTPClient: newPublicAlertHTTPClient(),
		events:           make(chan model.AlertEvent, 128),
	}
	svc.wg.Add(1)
	go svc.alertWorker()
	return svc, nil
}

func (svc *AlertService) Close() {
	svc.mu.Lock()
	if svc.closed {
		svc.mu.Unlock()
		return
	}
	svc.closed = true
	svc.cancel()
	svc.mu.Unlock()
	svc.wg.Wait()
}

func (svc *AlertService) ListChannels() ([]model.AlertChannel, error) {
	logging.Info("alert.channel.list", "读取告警渠道")
	return svc.store.ListAlertChannels()
}

func (svc *AlertService) CreateChannel(request model.AlertChannelRequest) (*model.AlertChannel, error) {
	item, err := svc.buildChannel(request, nil)
	if err != nil {
		return nil, err
	}
	if err := svc.store.CreateAlertChannel(item); err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	logging.Info("alert.channel.create", "创建告警渠道: id=%d type=%s", item.ID, item.Type)
	return svc.store.GetAlertChannel(item.ID)
}

func (svc *AlertService) UpdateChannel(id int64, request model.AlertChannelRequest) (*model.AlertChannel, error) {
	existing, err := svc.store.GetAlertChannel(id)
	if err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	item, err := svc.buildChannel(request, existing)
	if err != nil {
		return nil, err
	}
	item.ID, item.CreatedAt = id, existing.CreatedAt
	if err := svc.store.UpdateAlertChannel(item); err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	logging.Info("alert.channel.update", "更新告警渠道: id=%d type=%s", id, item.Type)
	return svc.store.GetAlertChannel(id)
}

func (svc *AlertService) DeleteChannel(id int64) (*model.ActionResponse, error) {
	if err := svc.store.DeleteAlertChannel(id); err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	logging.Info("alert.channel.delete", "删除告警渠道: id=%d", id)
	return &model.ActionResponse{Success: true, Message: "alert channel deleted"}, nil
}

func (svc *AlertService) buildChannel(request model.AlertChannelRequest, existing *model.AlertChannel) (*model.AlertChannel, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.Type = strings.TrimSpace(request.Type)
	if request.Name == "" || utf8.RuneCountInString(request.Name) > 100 || strings.ContainsAny(request.Name, "\r\n\x00") {
		return nil, fmt.Errorf("%w: 渠道名称不能为空且不能超过 100 个字符", ErrAlertInvalid)
	}
	item := &model.AlertChannel{Name: request.Name, Type: request.Type, Enabled: request.Enabled, Config: request.Config}
	if existing != nil {
		item.LastStatus = existing.LastStatus
		item.LastError = existing.LastError
		item.LastDeliveredAt = existing.LastDeliveredAt
	}

	requiredSecret := true
	switch item.Type {
	case model.AlertChannelWebhook:
		item.Config = model.AlertChannelConfig{WebhookAllowPrivate: request.Config.WebhookAllowPrivate}
	case model.AlertChannelTelegram:
		item.Config = model.AlertChannelConfig{TelegramChatID: strings.TrimSpace(request.Config.TelegramChatID)}
		if item.Config.TelegramChatID == "" || utf8.RuneCountInString(item.Config.TelegramChatID) > 100 || strings.ContainsAny(item.Config.TelegramChatID, "\r\n\x00") {
			return nil, fmt.Errorf("%w: Telegram Chat ID 不能为空且不能超过 100 个字符", ErrAlertInvalid)
		}
		item.Destination = "Telegram " + item.Config.TelegramChatID
	case model.AlertChannelEmail:
		config, err := normalizeAlertEmailConfig(request.Config)
		if err != nil {
			return nil, err
		}
		item.Config = config
		item.Destination = config.SMTPHost + ":" + strconv.Itoa(config.SMTPPort)
		requiredSecret = config.SMTPUsername != ""
		if existing != nil && existing.Type == model.AlertChannelEmail && config.SMTPUsername != existing.Config.SMTPUsername && config.SMTPUsername != "" && request.Secret == "" {
			return nil, fmt.Errorf("%w: 更改 SMTP 用户名时必须提供新的密码", ErrAlertInvalid)
		}
	default:
		return nil, fmt.Errorf("%w: 渠道类型必须是 webhook、telegram 或 email", ErrAlertInvalid)
	}

	if request.Secret == "" && existing != nil && existing.Type == item.Type && existing.HasSecret {
		item.SecretContext = existing.SecretContext
		item.SecretCiphertext = append([]byte(nil), existing.SecretCiphertext...)
		item.SecretNonce = append([]byte(nil), existing.SecretNonce...)
		item.HasSecret = true
		if item.Type == model.AlertChannelWebhook {
			plaintext, err := svc.cipher.decrypt("alert-"+existing.SecretContext, existing.Type, existing.SecretCiphertext, existing.SecretNonce)
			if err != nil {
				return nil, errors.New("Webhook 连接秘密无法解密")
			}
			if err := validateAlertChannelSecret(item.Type, string(plaintext), item.Config); err != nil {
				return nil, err
			}
			item.Destination = "地址已加密"
		}
	} else if request.Secret == "" {
		if requiredSecret {
			return nil, fmt.Errorf("%w: 当前渠道必须提供新的连接秘密", ErrAlertInvalid)
		}
	} else {
		if err := validateAlertChannelSecret(item.Type, request.Secret, item.Config); err != nil {
			return nil, err
		}
		contextID, err := randomHex(16)
		if err != nil {
			return nil, fmt.Errorf("%w: 无法生成渠道加密上下文", ErrAlertInvalid)
		}
		item.SecretContext = contextID
		item.SecretCiphertext, item.SecretNonce, err = svc.cipher.encrypt("alert-"+contextID, item.Type, []byte(request.Secret))
		if err != nil {
			return nil, fmt.Errorf("加密告警渠道秘密: %w", err)
		}
		item.HasSecret = true
		if item.Type == model.AlertChannelWebhook {
			item.Destination = "地址已加密"
		}
	}
	if item.Type == model.AlertChannelEmail && item.Config.SMTPUsername == "" && request.Secret == "" {
		item.SecretContext, item.SecretCiphertext, item.SecretNonce, item.HasSecret = "", nil, nil, false
	}
	return item, nil
}

func validateAlertChannelSecret(channelType, secret string, config model.AlertChannelConfig) error {
	if len(secret) > 8192 || strings.ContainsRune(secret, '\x00') {
		return fmt.Errorf("%w: 渠道秘密过长或包含无效字符", ErrAlertInvalid)
	}
	switch channelType {
	case model.AlertChannelWebhook:
		parsed, err := url.ParseRequestURI(secret)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
			return fmt.Errorf("%w: Webhook 地址必须是无用户信息和片段的 HTTP(S) URL", ErrAlertInvalid)
		}
		hostname := parsed.Hostname()
		if !config.WebhookAllowPrivate {
			if address, err := netip.ParseAddr(hostname); err == nil {
				if isPrivateAlertAddress(address) {
					return fmt.Errorf("%w: Webhook 私网地址需要显式允许内网目标", ErrAlertInvalid)
				}
			} else {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				addresses, lookupErr := net.DefaultResolver.LookupNetIP(ctx, "ip", hostname)
				cancel()
				if lookupErr != nil || len(addresses) == 0 {
					return fmt.Errorf("%w: Webhook 目标域名无法解析", ErrAlertInvalid)
				}
				for _, address := range addresses {
					if isPrivateAlertAddress(address) {
						return fmt.Errorf("%w: Webhook 域名解析到内网地址，需要显式允许内网目标", ErrAlertInvalid)
					}
				}
			}
		}
	case model.AlertChannelTelegram:
		if !telegramTokenPattern.MatchString(secret) {
			return fmt.Errorf("%w: Telegram Bot Token 格式无效", ErrAlertInvalid)
		}
	case model.AlertChannelEmail:
		if config.SMTPUsername == "" {
			return fmt.Errorf("%w: 使用 SMTP 密码时必须填写用户名", ErrAlertInvalid)
		}
	}
	return nil
}

func normalizeAlertEmailConfig(config model.AlertChannelConfig) (model.AlertChannelConfig, error) {
	config.TelegramChatID = ""
	config.SMTPHost = strings.TrimSpace(config.SMTPHost)
	config.SMTPUsername = strings.TrimSpace(config.SMTPUsername)
	config.SMTPFrom = strings.TrimSpace(config.SMTPFrom)
	config.SMTPTLSMode = strings.TrimSpace(config.SMTPTLSMode)
	if config.SMTPHost == "" || len(config.SMTPHost) > 253 || strings.ContainsAny(config.SMTPHost, "/\\@\r\n\x00") {
		return config, fmt.Errorf("%w: SMTP 主机无效", ErrAlertInvalid)
	}
	if _, err := netip.ParseAddr(config.SMTPHost); err != nil && strings.Contains(config.SMTPHost, ":") {
		return config, fmt.Errorf("%w: SMTP 主机不能包含端口", ErrAlertInvalid)
	}
	if config.SMTPPort < 1 || config.SMTPPort > 65535 {
		return config, fmt.Errorf("%w: SMTP 端口必须在 1 到 65535 之间", ErrAlertInvalid)
	}
	if config.SMTPTLSMode == "" {
		config.SMTPTLSMode = "starttls"
	}
	if config.SMTPTLSMode != "starttls" && config.SMTPTLSMode != "tls" && config.SMTPTLSMode != "none" {
		return config, fmt.Errorf("%w: SMTP 加密模式必须是 starttls、tls 或 none", ErrAlertInvalid)
	}
	if config.SMTPTLSMode == "none" && config.SMTPUsername != "" {
		return config, fmt.Errorf("%w: 未加密 SMTP 仅支持匿名投递", ErrAlertInvalid)
	}
	from, err := mail.ParseAddress(config.SMTPFrom)
	if err != nil || from.Address != config.SMTPFrom {
		return config, fmt.Errorf("%w: 发件人必须是有效的纯邮箱地址", ErrAlertInvalid)
	}
	if len(config.SMTPRecipients) == 0 || len(config.SMTPRecipients) > 20 {
		return config, fmt.Errorf("%w: 收件人数量必须在 1 到 20 之间", ErrAlertInvalid)
	}
	seen := make(map[string]bool)
	recipients := make([]string, 0, len(config.SMTPRecipients))
	for _, raw := range config.SMTPRecipients {
		value := strings.TrimSpace(raw)
		address, err := mail.ParseAddress(value)
		if err != nil || address.Address != value {
			return config, fmt.Errorf("%w: 收件人 %q 不是有效的纯邮箱地址", ErrAlertInvalid, value)
		}
		key := strings.ToLower(value)
		if !seen[key] {
			seen[key] = true
			recipients = append(recipients, value)
		}
	}
	sort.Strings(recipients)
	config.SMTPRecipients = recipients
	if len(config.SMTPUsername) > 320 || strings.ContainsAny(config.SMTPUsername, "\r\n\x00") {
		return config, fmt.Errorf("%w: SMTP 用户名无效", ErrAlertInvalid)
	}
	return config, nil
}

func (svc *AlertService) ListRules() ([]model.AlertRule, error) {
	logging.Info("alert.rule.list", "读取告警规则")
	return svc.store.ListAlertRules()
}

func (svc *AlertService) CreateRule(request model.AlertRuleRequest) (*model.AlertRule, error) {
	item, err := svc.normalizeRule(request)
	if err != nil {
		return nil, err
	}
	if err := svc.store.CreateAlertRule(item); err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	logging.Info("alert.rule.create", "创建告警规则: id=%d", item.ID)
	return svc.store.GetAlertRule(item.ID)
}

func (svc *AlertService) UpdateRule(id int64, request model.AlertRuleRequest) (*model.AlertRule, error) {
	existing, err := svc.store.GetAlertRule(id)
	if err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	item, err := svc.normalizeRule(request)
	if err != nil {
		return nil, err
	}
	item.ID, item.CreatedAt = id, existing.CreatedAt
	if err := svc.store.UpdateAlertRule(item); err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	logging.Info("alert.rule.update", "更新告警规则: id=%d", id)
	return svc.store.GetAlertRule(id)
}

func (svc *AlertService) DeleteRule(id int64) (*model.ActionResponse, error) {
	if err := svc.store.DeleteAlertRule(id); err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	logging.Info("alert.rule.delete", "删除告警规则: id=%d", id)
	return &model.ActionResponse{Success: true, Message: "alert rule deleted"}, nil
}

func (svc *AlertService) normalizeRule(request model.AlertRuleRequest) (*model.AlertRule, error) {
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || utf8.RuneCountInString(request.Name) > 100 || strings.ContainsAny(request.Name, "\r\n\x00") {
		return nil, fmt.Errorf("%w: 规则名称不能为空且不能超过 100 个字符", ErrAlertInvalid)
	}
	if request.CooldownMinutes < 0 || request.CooldownMinutes > 10080 {
		return nil, fmt.Errorf("%w: 去重冷却期必须在 0 到 10080 分钟之间", ErrAlertInvalid)
	}
	events := uniqueAlertEventTypes(request.EventTypes)
	if len(events) != uniqueStringCount(request.EventTypes) {
		return nil, fmt.Errorf("%w: 包含不支持的事件类型", ErrAlertInvalid)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("%w: 至少选择一种事件类型", ErrAlertInvalid)
	}
	channelIDs := uniquePositiveInt64s(request.ChannelIDs)
	if len(channelIDs) != uniqueInt64Count(request.ChannelIDs) {
		return nil, fmt.Errorf("%w: 渠道 ID 必须是正整数", ErrAlertInvalid)
	}
	if len(channelIDs) == 0 {
		return nil, fmt.Errorf("%w: 至少选择一个目标渠道", ErrAlertInvalid)
	}
	for _, id := range channelIDs {
		if _, err := svc.store.GetAlertChannel(id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%w: 渠道 %d 不存在", ErrAlertInvalid, id)
			}
			return nil, err
		}
	}
	return &model.AlertRule{
		Name: request.Name, Enabled: request.Enabled, EventTypes: events,
		ChannelIDs: channelIDs, CooldownMinutes: request.CooldownMinutes,
	}, nil
}

func uniqueAlertEventTypes(values []string) []string {
	allowed := map[string]bool{
		model.AlertEventCircuitOpen: true, model.AlertEventRecovered: true,
		model.AlertEventSubscriptionFailed: true,
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if allowed[value] && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]bool)
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value > 0 && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uniqueStringCount(values []string) int {
	seen := make(map[string]bool)
	for _, value := range values {
		seen[strings.TrimSpace(value)] = true
	}
	return len(seen)
}

func uniqueInt64Count(values []int64) int {
	seen := make(map[int64]bool)
	for _, value := range values {
		seen[value] = true
	}
	return len(seen)
}

func normalizeAlertStoreError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAlertNotFound
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "unique constraint") && (strings.Contains(lower, "alert_channels.name") || strings.Contains(lower, "alert_rules.name")) {
		return fmt.Errorf("%w: 名称已存在", ErrAlertConflict)
	}
	return err
}

func sanitizeAlertText(value string, maximum int) string {
	value = alertURLPattern.ReplaceAllString(value, "[URL]")
	value = alertSecretPattern.ReplaceAllString(value, "$1=[REDACTED]")
	value = strings.Join(strings.Fields(strings.ReplaceAll(value, "\x00", "")), " ")
	if maximum <= 0 || utf8.RuneCountInString(value) <= maximum {
		return value
	}
	runes := []rune(value)
	return string(runes[:maximum]) + "..."
}
