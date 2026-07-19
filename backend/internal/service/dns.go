package service

import (
	"fmt"
	"strings"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

type DNSService struct {
	store *store.Store
}

func NewDNSService(s *store.Store) *DNSService {
	return &DNSService{store: s}
}

// DNS Servers

func (svc *DNSService) ListDNSServers() ([]model.DNSServer, error) {
	return svc.store.ListDNSServers()
}

func (svc *DNSService) GetDNSServer(id int64) (*model.DNSServer, error) {
	return svc.store.GetDNSServer(id)
}

func (svc *DNSService) CreateDNSServer(req *model.DNSServerRequest) (*model.DNSServer, error) {
	return svc.store.CreateDNSServer(req)
}

func (svc *DNSService) UpdateDNSServer(id int64, req *model.DNSServerRequest) error {
	return svc.store.UpdateDNSServer(id, req)
}

func (svc *DNSService) DeleteDNSServer(id int64) error {
	return svc.store.DeleteDNSServer(id)
}

func (svc *DNSService) ReorderDNSServers(ids []int64) error {
	if len(ids) == 0 {
		return fmt.Errorf("DNS Server ID 不能为空")
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return fmt.Errorf("DNS Server ID 无效或重复")
		}
		seen[id] = true
	}
	logging.Info("dns.server.reorder", "调整 %d 个 DNS Server 的顺序", len(ids))
	return svc.store.ReorderDNSServers(ids)
}

func (svc *DNSService) GetDNSOutboundBindingOrder() (*model.DNSOutboundBindingOrder, error) {
	outbounds, err := svc.store.GetDNSOutboundBindingOrder()
	if err != nil {
		return nil, err
	}
	return &model.DNSOutboundBindingOrder{Outbounds: outbounds}, nil
}

func (svc *DNSService) SetDNSOutboundBindingOrder(req *model.DNSOutboundBindingOrder) error {
	if len(req.Outbounds) > 1000 {
		return fmt.Errorf("DNS 出口绑定顺序数量过多")
	}
	seen := make(map[string]bool, len(req.Outbounds))
	normalized := make([]string, 0, len(req.Outbounds))
	for _, outbound := range req.Outbounds {
		outbound = strings.TrimSpace(outbound)
		if outbound == "" || seen[outbound] {
			return fmt.Errorf("DNS 出口绑定顺序包含空值或重复项")
		}
		seen[outbound] = true
		normalized = append(normalized, outbound)
	}
	logging.Info("dns.outbound_binding.reorder", "调整 %d 个 DNS 出口绑定的显示顺序", len(normalized))
	return svc.store.SetDNSOutboundBindingOrder(normalized)
}

// DNS Rules

func (svc *DNSService) ListDNSRules() ([]model.DNSRule, error) {
	return svc.store.ListDNSRules()
}

func (svc *DNSService) GetDNSRule(id int64) (*model.DNSRule, error) {
	return svc.store.GetDNSRule(id)
}

func (svc *DNSService) CreateDNSRule(req *model.DNSRuleRequest) (*model.DNSRule, error) {
	return svc.store.CreateDNSRule(req)
}

func (svc *DNSService) UpdateDNSRule(id int64, req *model.DNSRuleRequest) error {
	return svc.store.UpdateDNSRule(id, req)
}

func (svc *DNSService) DeleteDNSRule(id int64) error {
	return svc.store.DeleteDNSRule(id)
}

func (svc *DNSService) ReorderDNSRules(ids []int64) error {
	return svc.store.ReorderDNSRules(ids)
}

// DNS Global Settings

func (svc *DNSService) GetDNSGlobalSettings() (*model.DNSGlobalSettings, error) {
	return svc.store.GetDNSGlobalSettings()
}

func (svc *DNSService) SetDNSGlobalSettings(req *model.DNSGlobalSettings) error {
	return svc.store.SetDNSGlobalSettings(req)
}
