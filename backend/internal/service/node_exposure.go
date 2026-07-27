package service

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

var (
	ErrNodeExposureNotFound = errors.New("节点暴露配置不存在")
	ErrNodeExposureConflict = errors.New("节点暴露配置冲突")
	ErrNodeExposureInvalid  = errors.New("节点暴露配置无效")
	ErrNodeExposureApply    = errors.New("节点暴露配置应用失败")
)

type NodeExposureService struct {
	store   *store.Store
	runtime nodeExposureRuntime
	core    nodeExposureCore
	mu      sync.Mutex
}

type nodeExposureCore interface {
	IsRunning() bool
}

func NewNodeExposureService(store *store.Store) *NodeExposureService {
	return &NodeExposureService{store: store}
}

func (s *NodeExposureService) SetRuntimeDependencies(runtime nodeExposureRuntime, core nodeExposureCore) {
	s.runtime = runtime
	s.core = core
}

func (s *NodeExposureService) List() ([]model.NodeExposureWithNode, error) {
	logging.Info("node_exposure.list", "读取节点暴露配置")
	return s.store.ListNodeExposures()
}

func (s *NodeExposureService) Create(req model.NodeExposureRequest) (*model.NodeExposureWithNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, err := s.normalize(req, nil)
	if err != nil {
		return nil, err
	}
	if err := s.validateRuntimeConflicts(item, 0); err != nil {
		return nil, err
	}
	logging.Info("node_exposure.create", "创建节点暴露: %s, %s://%s:%d", item.Name, item.InboundType, item.Listen, item.ListenPort)
	if err := s.mutate(func() error { return s.store.CreateNodeExposure(item) }); err != nil {
		return nil, normalizeNodeExposureStoreError(err)
	}
	created, err := s.store.GetNodeExposure(item.ID)
	if err != nil {
		if rollbackErr := s.mutate(func() error { return s.store.DeleteNodeExposure(item.ID) }); rollbackErr != nil {
			return nil, fmt.Errorf("读取新建节点暴露失败: %v；回滚数据库也失败: %w", err, rollbackErr)
		}
		return nil, normalizeNodeExposureStoreError(err)
	}
	if err := s.applyRuntimeMutation(func() error {
		return s.applyRuntimeItem(*created)
	}, func() error {
		return s.mutate(func() error { return s.store.DeleteNodeExposure(item.ID) })
	}, func() error {
		return s.runtime.Delete(item.ID)
	}, item.Password); err != nil {
		return nil, err
	}
	logging.Info("node_exposure.create", "节点暴露已应用: %d", item.ID)
	return s.store.GetNodeExposure(item.ID)
}

func (s *NodeExposureService) Update(id int64, req model.NodeExposureRequest) (*model.NodeExposureWithNode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := s.store.GetNodeExposure(id)
	if err != nil {
		return nil, normalizeNodeExposureStoreError(err)
	}
	item, err := s.normalize(req, existing)
	if err != nil {
		return nil, err
	}
	if err := s.validateRuntimeConflicts(item, id); err != nil {
		return nil, err
	}
	logging.Info("node_exposure.update", "更新节点暴露: %d, %s://%s:%d", id, item.InboundType, item.Listen, item.ListenPort)
	if err := s.mutate(func() error { return s.store.UpdateNodeExposure(id, item) }); err != nil {
		return nil, normalizeNodeExposureStoreError(err)
	}
	updated, err := s.store.GetNodeExposure(id)
	if err != nil {
		if rollbackErr := s.mutate(func() error { return s.store.RestoreNodeExposure(&existing.NodeExposure) }); rollbackErr != nil {
			return nil, fmt.Errorf("读取更新后的节点暴露失败: %v；回滚数据库也失败: %w", err, rollbackErr)
		}
		return nil, normalizeNodeExposureStoreError(err)
	}
	if err := s.applyRuntimeMutation(func() error {
		return s.applyRuntimeItem(*updated)
	}, func() error {
		return s.mutate(func() error { return s.store.RestoreNodeExposure(&existing.NodeExposure) })
	}, func() error {
		return s.applyRuntimeItem(*existing)
	}, item.Password, existing.Password); err != nil {
		return nil, err
	}
	logging.Info("node_exposure.update", "节点暴露已应用: %d", id)
	return s.store.GetNodeExposure(id)
}

func (s *NodeExposureService) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, err := s.store.GetNodeExposure(id)
	if err != nil {
		return normalizeNodeExposureStoreError(err)
	}
	logging.Info("node_exposure.delete", "删除节点暴露: %d", id)
	if err := s.mutate(func() error { return s.store.DeleteNodeExposure(id) }); err != nil {
		return normalizeNodeExposureStoreError(err)
	}
	if err := s.applyRuntimeMutation(func() error {
		return s.runtime.Delete(id)
	}, func() error {
		return s.mutate(func() error { return s.store.RestoreNodeExposure(&existing.NodeExposure) })
	}, func() error {
		return s.applyRuntimeItem(*existing)
	}, existing.Password); err != nil {
		return err
	}
	logging.Info("node_exposure.delete", "节点暴露已删除并应用: %d", id)
	return nil
}

func (s *NodeExposureService) normalize(req model.NodeExposureRequest, existing *model.NodeExposureWithNode) (*model.NodeExposure, error) {
	item := &model.NodeExposure{
		Name:           strings.TrimSpace(req.Name),
		SubscriptionID: req.SubscriptionID,
		NodeUID:        strings.TrimSpace(req.NodeUID),
		InboundType:    strings.ToLower(strings.TrimSpace(req.InboundType)),
		Listen:         strings.TrimSpace(req.Listen),
		ListenPort:     req.ListenPort,
		Username:       strings.TrimSpace(req.Username),
		Password:       req.Password,
		Enabled:        req.Enabled,
	}
	if item.Listen == "" {
		item.Listen = "127.0.0.1"
	}
	if existing != nil && item.Password == "" && !req.ClearPassword {
		item.Password = existing.Password
	}
	if req.ClearPassword {
		item.Password = ""
	}
	if item.Name == "" {
		return nil, fmt.Errorf("%w: 名称不能为空", ErrNodeExposureInvalid)
	}
	if len([]rune(item.Name)) > 100 {
		return nil, fmt.Errorf("%w: 名称不能超过 100 个字符", ErrNodeExposureInvalid)
	}
	if item.SubscriptionID <= 0 || item.NodeUID == "" {
		return nil, fmt.Errorf("%w: 必须选择目标节点", ErrNodeExposureInvalid)
	}
	switch item.InboundType {
	case "http", "socks", "mixed":
	default:
		return nil, fmt.Errorf("%w: 入站协议必须是 http、socks 或 mixed", ErrNodeExposureInvalid)
	}
	ip := net.ParseIP(item.Listen)
	if ip == nil {
		return nil, fmt.Errorf("%w: 监听地址必须是 IP 地址", ErrNodeExposureInvalid)
	}
	item.Listen = ip.String()
	if item.ListenPort < 1 || item.ListenPort > 65535 {
		return nil, fmt.Errorf("%w: 监听端口必须在 1 到 65535 之间", ErrNodeExposureInvalid)
	}
	hasUsername := item.Username != ""
	if item.Password != "" && strings.TrimSpace(item.Password) == "" {
		return nil, fmt.Errorf("%w: 密码不能只包含空白字符", ErrNodeExposureInvalid)
	}
	if utf8.RuneCountInString(item.Username) > 64 || strings.ContainsAny(item.Username, "\r\n\x00") {
		return nil, fmt.Errorf("%w: 用户名必须是 64 个字符以内的单行文本", ErrNodeExposureInvalid)
	}
	if utf8.RuneCountInString(item.Password) > 128 || strings.ContainsAny(item.Password, "\r\n\x00") {
		return nil, fmt.Errorf("%w: 密码必须是 128 个字符以内的单行文本", ErrNodeExposureInvalid)
	}
	hasPassword := item.Password != ""
	if hasUsername != hasPassword {
		return nil, fmt.Errorf("%w: 用户名和密码必须同时设置", ErrNodeExposureInvalid)
	}
	if !ip.IsLoopback() && (!hasUsername || !hasPassword) {
		return nil, fmt.Errorf("%w: 非回环地址监听必须设置用户名和密码", ErrNodeExposureInvalid)
	}

	node, err := s.store.GetNodeByReference(item.SubscriptionID, item.NodeUID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) && existing != nil && !item.Enabled &&
			existing.SubscriptionID == item.SubscriptionID && existing.NodeUID == item.NodeUID {
			return item, nil
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: 目标节点不存在", ErrNodeExposureInvalid)
		}
		return nil, err
	}
	if item.Enabled && !node.Enabled {
		return nil, fmt.Errorf("%w: 目标节点已停用，不能启用暴露", ErrNodeExposureInvalid)
	}
	return item, nil
}

func (s *NodeExposureService) applyRuntimeMutation(apply, rollback, restore func() error, secrets ...string) error {
	if s.runtime == nil || s.core == nil || !s.core.IsRunning() {
		return nil
	}
	applyErr := apply()
	if applyErr == nil {
		return nil
	}
	applyErr = s.redactError(applyErr, secrets...)
	logging.Error("node_exposure.apply", "%v", applyErr)
	if rollbackErr := rollback(); rollbackErr != nil {
		return fmt.Errorf("%w: %v；回滚数据库也失败: %w", ErrNodeExposureApply, applyErr, rollbackErr)
	}
	if restoreErr := restore(); restoreErr != nil {
		restoreErr = s.redactError(restoreErr, secrets...)
		return fmt.Errorf("%w: %v；数据库已回滚，但恢复运行时失败: %w", ErrNodeExposureApply, applyErr, restoreErr)
	}
	return fmt.Errorf("%w，已回滚: %w", ErrNodeExposureApply, applyErr)
}

func (s *NodeExposureService) applyRuntimeItem(item model.NodeExposureWithNode) error {
	if item.Enabled {
		return s.runtime.Upsert(item)
	}
	return s.runtime.Delete(item.ID)
}

func (s *NodeExposureService) SyncRuntime() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runtime == nil {
		return nil
	}
	items, err := s.store.ListNodeExposures()
	if err != nil {
		return err
	}
	logging.Info("node_exposure.sync", "同步 %d 项节点暴露到核心运行时", len(items))
	if err := s.runtime.Sync(items); err != nil {
		redactedErr := s.redactError(err)
		logging.Error("node_exposure.sync", "同步节点暴露失败: %v", redactedErr)
		return redactedErr
	}
	logging.Info("node_exposure.sync", "节点暴露运行时同步完成")
	return nil
}

func (s *NodeExposureService) mutate(operation func() error) error {
	release := s.store.HoldConfigUpdate()
	defer release()
	return operation()
}

func (s *NodeExposureService) redactError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, secret := range secrets {
		if secret != "" {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
	}
	items, listErr := s.store.ListNodeExposures()
	if listErr == nil {
		for _, item := range items {
			if item.Password != "" {
				message = strings.ReplaceAll(message, item.Password, "[REDACTED]")
			}
		}
	}
	return errors.New(message)
}

func (s *NodeExposureService) validateRuntimeConflicts(item *model.NodeExposure, excludeID int64) error {
	if !item.Enabled {
		return nil
	}
	items, err := s.store.ListNodeExposures()
	if err != nil {
		return err
	}
	for _, other := range items {
		if !other.Enabled || other.ID == excludeID || other.ListenPort != item.ListenPort {
			continue
		}
		if listenersOverlap(item.Listen, other.Listen) {
			return fmt.Errorf("%w: 监听端口与 %q 冲突", ErrNodeExposureConflict, other.Name)
		}
	}
	for port, owner := range reservedNodeExposurePorts(s.store) {
		if item.ListenPort == port {
			return fmt.Errorf("%w: 监听端口 %d 已被%s使用", ErrNodeExposureConflict, port, owner)
		}
	}
	return nil
}

func listenersOverlap(left, right string) bool {
	leftIP, rightIP := net.ParseIP(left), net.ParseIP(right)
	if leftIP == nil || rightIP == nil {
		return left == right
	}
	return leftIP.IsUnspecified() || rightIP.IsUnspecified() || leftIP.Equal(rightIP)
}

func reservedNodeExposurePorts(db *store.Store) map[int]string {
	ports := map[int]string{
		defaultDNSInboundPort: "Ackwrap DNS 入站",
		singboxRuntimeAPIPort: "核心运行时 API",
	}
	if request, err := db.GetConfigGenerateRequest(); err == nil && request != nil && request.InboundPort > 0 {
		ports[request.InboundPort] = "Mixed 入站"
	} else {
		ports[model.DefaultMixedInboundPort] = "默认 Mixed 入站"
	}
	if settings, err := db.GetExperimentalSettings(); err == nil && settings != nil {
		if port, err := strconv.Atoi(settings.ClashAPIPort); err == nil && port > 0 && port <= 65535 {
			ports[port] = "Clash API"
		}
	}
	listenAddr := strings.TrimSpace(os.Getenv("ACKWRAP_LISTEN_ADDR"))
	if listenAddr == "" {
		ports[8080] = "Ackwrap 管理端"
	} else {
		if _, value, err := net.SplitHostPort(listenAddr); err == nil {
			if port, err := strconv.Atoi(value); err == nil && port > 0 && port <= 65535 {
				ports[port] = "Ackwrap 管理端"
			}
		}
	}
	return ports
}

func normalizeNodeExposureStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNodeExposureNotFound
	}
	if errors.Is(err, store.ErrNodeExposureTargetUnavailable) {
		return fmt.Errorf("%w: 目标节点不存在或已停用", ErrNodeExposureInvalid)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") {
		return fmt.Errorf("%w: 名称或监听地址已被使用", ErrNodeExposureConflict)
	}
	return err
}
