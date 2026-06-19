package service

import (
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
