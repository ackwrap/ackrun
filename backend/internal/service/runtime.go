package service

import (
	"os"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

type RuntimeService struct {
	paths   *paths.Paths
	store   *store.Store
	singbox *SingboxService
}

func NewRuntimeService(p *paths.Paths, s *store.Store, sb *SingboxService) *RuntimeService {
	return &RuntimeService{paths: p, store: s, singbox: sb}
}

func (svc *RuntimeService) GetStatus() (*model.RuntimeResponse, error) {
	logging.Info("runtime.check", "checking runtime status")

	resp := &model.RuntimeResponse{}

	if _, err := os.Stat(svc.paths.BinaryPath); os.IsNotExist(err) {
		logging.Info("runtime.check", "binary not found: %s", svc.paths.BinaryPath)
		resp.Status = model.RuntimeNotInstalled
		return resp, nil
	}

	resp.Version = svc.getVersion()

	if _, ok, err := svc.paths.ActiveConfigPath(); err != nil {
		return nil, err
	} else if !ok {
		logging.Info("runtime.check", "config not found in: %s", svc.paths.ConfigDir)
		resp.Status = model.RuntimeNoConfig
		return resp, nil
	}

	pid := svc.singbox.GetPID()
	if pid > 0 {
		resp.Status = model.RuntimeRunning
		resp.PID = pid
		return resp, nil
	}

	resp.Status = model.RuntimeStopped
	return resp, nil
}

var cachedVersion string

func (svc *RuntimeService) getVersion() string {
	if cachedVersion != "" {
		return cachedVersion
	}

	installState, err := svc.store.GetInstallState()
	if err == nil && isSingboxVersion(installState.Version) {
		cachedVersion = installState.Version
		return cachedVersion
	}

	cachedVersion = readSingboxVersion(svc.paths.BinaryPath)
	return cachedVersion
}
