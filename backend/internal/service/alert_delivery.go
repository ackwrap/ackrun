package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/httpclient"
	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

type alertHTTPClient interface {
	Do(request *http.Request) (*http.Response, error)
}

func newAlertHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func newPublicAlertHTTPClient() *http.Client {
	client := newAlertHTTPClient()
	transport := client.Transport.(*http.Transport)
	dialer := &net.Dialer{}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("Webhook 目标地址无效")
		}
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, errors.New("Webhook 目标域名解析失败")
		}
		for _, candidate := range addresses {
			if isPrivateAlertAddress(candidate) {
				continue
			}
			connection, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
			if err == nil {
				return connection, nil
			}
		}
		return nil, errors.New("Webhook 目标没有可用的公网地址")
	}
	return client
}

func (svc *AlertService) Notify(event model.AlertEvent) {
	event.Type = strings.TrimSpace(event.Type)
	if !isRuleAlertEvent(event.Type) {
		return
	}
	event.TargetKey = sanitizeAlertText(event.TargetKey, 200)
	event.SourceName = sanitizeAlertText(event.SourceName, 200)
	event.Title = sanitizeAlertText(event.Title, 200)
	event.Message = sanitizeAlertText(event.Message, 1000)
	if event.OccurredAt <= 0 {
		event.OccurredAt = svc.now().UTC().UnixMilli()
	}
	if event.TargetKey == "" {
		event.TargetKey = event.SourceName
	}

	svc.mu.Lock()
	if svc.closed {
		svc.mu.Unlock()
		return
	}
	select {
	case svc.events <- event:
	default:
		logging.Error("alert.event", "告警事件队列已满，丢弃事件: type=%s", event.Type)
	}
	svc.mu.Unlock()
}

func (svc *AlertService) alertWorker() {
	defer svc.wg.Done()
	for {
		select {
		case <-svc.ctx.Done():
			return
		case event := <-svc.events:
			svc.dispatchEvent(svc.ctx, event)
		}
	}
}

func (svc *AlertService) dispatchEvent(ctx context.Context, event model.AlertEvent) {
	rules, err := svc.store.ListAlertRules()
	if err != nil {
		logging.Error("alert.event", "读取事件规则失败: type=%s err=%v", event.Type, err)
		return
	}
	channels, err := svc.store.ListAlertChannels()
	if err != nil {
		logging.Error("alert.event", "读取事件渠道失败: type=%s err=%v", event.Type, err)
		return
	}
	channelByID := make(map[int64]*model.AlertChannel, len(channels))
	for index := range channels {
		channelByID[channels[index].ID] = &channels[index]
	}
	for index := range rules {
		rule := &rules[index]
		if !rule.Enabled || !containsString(rule.EventTypes, event.Type) {
			continue
		}
		targets := make([]*model.AlertChannel, 0, len(rule.ChannelIDs))
		for _, channelID := range rule.ChannelIDs {
			if channel := channelByID[channelID]; channel != nil && channel.Enabled {
				targets = append(targets, channel)
			}
		}
		if len(targets) == 0 {
			continue
		}
		dedupKey := event.Type + ":" + event.TargetKey
		claimed, err := svc.store.ClaimAlertRuleCooldown(
			rule.ID, dedupKey, svc.now().UTC().UnixMilli(), time.Duration(rule.CooldownMinutes)*time.Minute,
		)
		if err != nil {
			logging.Error("alert.event", "更新告警冷却状态失败: rule_id=%d err=%v", rule.ID, err)
			continue
		}
		if !claimed {
			logging.Info("alert.event", "事件处于冷却期: rule_id=%d type=%s", rule.ID, event.Type)
			continue
		}
		for _, channel := range targets {
			if ctx.Err() != nil {
				return
			}
			ruleID := rule.ID
			if _, err := svc.deliverAndRecord(ctx, channel, &ruleID, event, false); err != nil {
				logging.Error("alert.delivery.send", "告警投递失败: channel_id=%d type=%s", channel.ID, event.Type)
			}
		}
	}
}

func (svc *AlertService) TestChannel(ctx context.Context, id int64) (*model.AlertDelivery, error) {
	channel, err := svc.store.GetAlertChannel(id)
	if err != nil {
		return nil, normalizeAlertStoreError(err)
	}
	logging.Info("alert.channel.test", "测试告警渠道: id=%d type=%s", id, channel.Type)
	event := model.AlertEvent{
		Type: model.AlertEventTest, TargetKey: "channel:" + strconv.FormatInt(id, 10),
		SourceName: channel.Name, Title: "[Ackwrap] 告警渠道测试",
		Message: "这是一条 Ackwrap 告警渠道测试消息。", OccurredAt: svc.now().UTC().UnixMilli(),
	}
	delivery, deliveryErr := svc.deliverAndRecord(ctx, channel, nil, event, true)
	if deliveryErr != nil {
		return delivery, fmt.Errorf("%w: %s", ErrAlertDelivery, delivery.Error)
	}
	return delivery, nil
}

func (svc *AlertService) ListDeliveries(page, pageSize int, channelID int64, eventType, status string) (*model.AlertDeliveryPage, error) {
	if page < 1 || pageSize < 1 || pageSize > 200 {
		return nil, fmt.Errorf("%w: page 必须大于 0，page_size 必须在 1 到 200 之间", ErrAlertInvalid)
	}
	eventType, status = strings.TrimSpace(eventType), strings.TrimSpace(status)
	if eventType != "" && !isDeliveryAlertEvent(eventType) {
		return nil, fmt.Errorf("%w: 事件类型筛选无效", ErrAlertInvalid)
	}
	if status != "" && status != model.AlertDeliverySuccess && status != model.AlertDeliveryFailed {
		return nil, fmt.Errorf("%w: 状态筛选必须是 success 或 failed", ErrAlertInvalid)
	}
	logging.Info("alert.delivery.list", "读取告警投递记录")
	result, err := svc.store.ListAlertDeliveries(model.AlertDeliveryFilter{
		ChannelID: channelID, EventType: eventType, Status: status,
		Limit: pageSize, Offset: (page - 1) * pageSize,
	})
	if err != nil {
		return nil, err
	}
	result.Page = page
	return result, nil
}

func (svc *AlertService) ClearDeliveries() (*model.ActionResponse, error) {
	logging.Info("alert.delivery.clear", "清空告警投递记录")
	if err := svc.store.ClearAlertDeliveries(); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "alert deliveries cleared"}, nil
}

func (svc *AlertService) deliverAndRecord(ctx context.Context, channel *model.AlertChannel, ruleID *int64, event model.AlertEvent, isTest bool) (*model.AlertDelivery, error) {
	secret, err := svc.decryptChannelSecret(channel)
	statusCode := 0
	if err == nil {
		statusCode, err = svc.deliver(ctx, channel, secret, event)
	}
	deliveredAt := svc.now().UTC().UnixMilli()
	errorMessage := ""
	if err != nil {
		errorMessage = sanitizeAlertText(err.Error(), 500)
		if errorMessage == "" {
			errorMessage = "投递失败"
		}
	}
	channelID := channel.ID
	delivery := &model.AlertDelivery{
		RuleID: ruleID, ChannelID: &channelID, ChannelName: channel.Name, ChannelType: channel.Type,
		EventType: event.Type, EventTitle: event.Title, Success: err == nil,
		StatusCode: statusCode, Error: errorMessage, IsTest: isTest, DeliveredAt: deliveredAt,
	}
	if recordErr := svc.store.CreateAlertDelivery(delivery); recordErr != nil {
		return delivery, fmt.Errorf("record alert delivery: %w", recordErr)
	}
	if _, pruneErr := svc.store.PruneAlertDeliveries(10000); pruneErr != nil {
		logging.Error("alert.delivery.cleanup", "清理告警投递记录失败: %v", pruneErr)
	}
	status := model.AlertDeliverySuccess
	if err != nil {
		status = model.AlertDeliveryFailed
	}
	if stateErr := svc.store.UpdateAlertChannelDeliveryState(channel.ID, status, errorMessage, deliveredAt); stateErr != nil {
		return delivery, fmt.Errorf("update alert channel state: %w", stateErr)
	}
	if err != nil {
		return delivery, err
	}
	logging.Info("alert.delivery.send", "告警投递成功: channel_id=%d type=%s", channel.ID, event.Type)
	return delivery, nil
}

func (svc *AlertService) decryptChannelSecret(channel *model.AlertChannel) (string, error) {
	if !channel.HasSecret {
		if channel.Type == model.AlertChannelEmail && channel.Config.SMTPUsername == "" {
			return "", nil
		}
		return "", errors.New("渠道连接秘密未配置")
	}
	plaintext, err := svc.cipher.decrypt("alert-"+channel.SecretContext, channel.Type, channel.SecretCiphertext, channel.SecretNonce)
	if err != nil {
		return "", errors.New("渠道连接秘密无法解密")
	}
	return string(plaintext), nil
}

func (svc *AlertService) deliver(ctx context.Context, channel *model.AlertChannel, secret string, event model.AlertEvent) (int, error) {
	switch channel.Type {
	case model.AlertChannelWebhook:
		return svc.sendWebhook(ctx, secret, channel.Config.WebhookAllowPrivate, event)
	case model.AlertChannelTelegram:
		return svc.sendTelegram(ctx, secret, channel.Config.TelegramChatID, event)
	case model.AlertChannelEmail:
		return sendAlertEmail(ctx, channel.Config, secret, event)
	default:
		return 0, errors.New("不支持的告警渠道类型")
	}
}

func (svc *AlertService) sendWebhook(ctx context.Context, endpoint string, allowPrivate bool, event model.AlertEvent) (int, error) {
	payload, err := json.Marshal(map[string]any{"event": event, "source": "ackwrap"})
	if err != nil {
		return 0, errors.New("无法编码 Webhook 请求")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, errors.New("无法创建 Webhook 请求")
	}
	request.Header.Set("Content-Type", "application/json")
	httpclient.SetBrowserUserAgent(request)
	client := svc.publicHTTPClient
	if allowPrivate {
		client = svc.httpClient
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, errors.New("Webhook 请求失败")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("Webhook 返回 HTTP %d", response.StatusCode)
	}
	return response.StatusCode, nil
}

func (svc *AlertService) sendTelegram(ctx context.Context, token, chatID string, event model.AlertEvent) (int, error) {
	payload, err := json.Marshal(map[string]any{
		"chat_id": chatID,
		"text":    event.Title + "\n\n" + event.Message,
	})
	if err != nil {
		return 0, errors.New("无法编码 Telegram 请求")
	}
	endpoint := strings.TrimRight(svc.telegramBaseURL, "/") + "/bot" + token + "/sendMessage"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, errors.New("无法创建 Telegram 请求")
	}
	request.Header.Set("Content-Type", "application/json")
	httpclient.SetBrowserUserAgent(request)
	response, err := svc.httpClient.Do(request)
	if err != nil {
		return 0, errors.New("Telegram API 请求失败")
	}
	defer response.Body.Close()
	var result struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
	if response.StatusCode < 200 || response.StatusCode >= 300 || decodeErr != nil || !result.OK {
		description := sanitizeAlertText(result.Description, 200)
		if description == "" {
			description = "Telegram API 返回无效响应"
		}
		return response.StatusCode, fmt.Errorf("%s (code %d)", description, result.ErrorCode)
	}
	return response.StatusCode, nil
}

func sendAlertEmail(ctx context.Context, config model.AlertChannelConfig, password string, event model.AlertEvent) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	address := net.JoinHostPort(config.SMTPHost, strconv.Itoa(config.SMTPPort))
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return 0, errors.New("SMTP 连接失败")
	}
	rawConnection := connection
	defer rawConnection.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = rawConnection.Close()
		case <-done:
		}
	}()
	deadline, _ := ctx.Deadline()
	_ = rawConnection.SetDeadline(deadline)
	tlsConfig := &tls.Config{ServerName: config.SMTPHost, MinVersion: tls.VersionTLS12}
	if config.SMTPTLSMode == "tls" {
		tlsConnection := tls.Client(connection, tlsConfig)
		if err := tlsConnection.HandshakeContext(ctx); err != nil {
			return 0, errors.New("SMTP TLS 握手失败")
		}
		connection = tlsConnection
	}
	client, err := smtp.NewClient(connection, config.SMTPHost)
	if err != nil {
		return 0, errors.New("SMTP 协议握手失败")
	}
	defer client.Close()
	if config.SMTPTLSMode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return 0, errors.New("SMTP 服务器不支持 STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return 0, errors.New("SMTP STARTTLS 握手失败")
		}
	}
	if config.SMTPUsername != "" {
		if ok, _ := client.Extension("AUTH"); !ok {
			return 0, errors.New("SMTP 服务器不支持认证")
		}
		if err := client.Auth(smtp.PlainAuth("", config.SMTPUsername, password, config.SMTPHost)); err != nil {
			return 0, errors.New("SMTP 认证失败")
		}
	}
	if err := client.Mail(config.SMTPFrom); err != nil {
		return 0, errors.New("SMTP 发件人被拒绝")
	}
	for _, recipient := range config.SMTPRecipients {
		if err := client.Rcpt(recipient); err != nil {
			return 0, errors.New("SMTP 收件人被拒绝")
		}
	}
	writer, err := client.Data()
	if err != nil {
		return 0, errors.New("SMTP 无法开始发送邮件内容")
	}
	subject := mime.QEncoding.Encode("UTF-8", sanitizeAlertText(event.Title, 180))
	message := strings.Join([]string{
		"Date: " + time.UnixMilli(event.OccurredAt).UTC().Format(time.RFC1123Z),
		"From: " + config.SMTPFrom,
		"To: " + strings.Join(config.SMTPRecipients, ", "),
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		event.Message,
	}, "\r\n")
	if _, err := io.WriteString(writer, message); err != nil {
		writer.Close()
		return 0, errors.New("SMTP 写入邮件内容失败")
	}
	if err := writer.Close(); err != nil {
		return 0, errors.New("SMTP 服务器拒绝邮件内容")
	}
	_ = client.Quit()
	return 250, nil
}

func isRuleAlertEvent(value string) bool {
	return value == model.AlertEventCircuitOpen || value == model.AlertEventRecovered || value == model.AlertEventSubscriptionFailed
}

func isDeliveryAlertEvent(value string) bool {
	return isRuleAlertEvent(value) || value == model.AlertEventTest
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isPrivateAlertAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() || address.IsMulticast() {
		return true
	}
	cgnat := netip.MustParsePrefix("100.64.0.0/10")
	return cgnat.Contains(address)
}
