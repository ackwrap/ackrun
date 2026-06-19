package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

type SettingsService struct {
	store *store.Store
}

func NewSettingsService(s *store.Store) *SettingsService {
	return &SettingsService{store: s}
}

func (svc *SettingsService) GetUpdateSettings() (*model.UpdateSettingsResponse, error) {
	return svc.store.GetUpdateSettings()
}

func (svc *SettingsService) SetUpdateSettings(req *model.UpdateSettings) error {
	return svc.store.SetUpdateSettings(req)
}

func (svc *SettingsService) GetLogSettings() (*model.LogSettingsResponse, error) {
	return svc.store.GetLogSettings()
}

func (svc *SettingsService) SetLogSettings(req *model.LogSettings) error {
	return svc.store.SetLogSettings(req)
}

func (svc *SettingsService) GetNTPSettings() (*model.NTPSettingsResponse, error) {
	return svc.store.GetNTPSettings()
}

func (svc *SettingsService) SetNTPSettings(req *model.NTPSettings) error {
	return svc.store.SetNTPSettings(req)
}

func (svc *SettingsService) GetDNSSettings() (*model.DNSSettingsResponse, error) {
	return svc.store.GetDNSSettings()
}

func (svc *SettingsService) SetDNSSettings(req *model.DNSSettings) error {
	return svc.store.SetDNSSettings(req)
}

func (svc *SettingsService) GetInboundMode() string {
	return svc.store.GetInboundMode()
}

func (svc *SettingsService) SetInboundMode(mode string) error {
	return svc.store.SetInboundMode(mode)
}

func (svc *SettingsService) GetExperimentalSettings() (*model.ExperimentalSettingsResponse, error) {
	return svc.store.GetExperimentalSettings()
}

func (svc *SettingsService) SetExperimentalSettings(req *model.ExperimentalSettings) error {
	return svc.store.SetExperimentalSettings(req)
}

func (svc *SettingsService) ListNodeFilters() ([]model.NodeFilter, error) {
	return svc.store.ListNodeFilters()
}

func (svc *SettingsService) CreateNodeFilter(req *model.NodeFilterRequest) (*model.NodeFilter, error) {
	if err := validateNodeFilter(req); err != nil {
		return nil, err
	}
	return svc.store.CreateNodeFilter(req)
}

func (svc *SettingsService) UpdateNodeFilter(id int64, req *model.NodeFilterRequest) (*model.NodeFilter, error) {
	if err := validateNodeFilter(req); err != nil {
		return nil, err
	}
	item, err := svc.store.UpdateNodeFilter(id, req)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("node filter not found")
	}
	return item, nil
}

func (svc *SettingsService) DeleteNodeFilter(id int64) (*model.ActionResponse, error) {
	if err := svc.store.DeleteNodeFilter(id); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "node filter deleted"}, nil
}

func validateNodeFilter(req *model.NodeFilterRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Target = strings.TrimSpace(req.Target)
	req.Pattern = strings.TrimSpace(req.Pattern)
	if req.Name == "" {
		return fmt.Errorf("filter name is required")
	}
	switch req.Target {
	case "all", "name", "type", "server", "raw", "raw_json":
	default:
		return fmt.Errorf("filter target must be all, name, type, server, raw, or raw_json")
	}
	if req.Pattern == "" {
		return fmt.Errorf("filter pattern is required")
	}
	if _, err := regexp.Compile(req.Pattern); err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	return nil
}
