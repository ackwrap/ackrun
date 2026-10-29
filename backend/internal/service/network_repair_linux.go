//go:build linux

package service

import (
	"os/exec"
	"strings"

	"github.com/ackwrap/ackrun/internal/paths"
)

func resetPlatformDeviceDNS(p *paths.Paths) error {
	return newDeviceDNSLifecycle(p).(*nftDeviceDNS).restore(true)
}

func resetPlatformTransitionGuard(p *paths.Paths) error {
	return newCoreTransitionGuard(p).(*nftTransitionGuard).disable(true)
}

func forceCleanupPlatformSingboxState(statePath string) (platformCleanupResult, error) {
	running, err := linuxSingboxProcessRunning()
	if err != nil || running {
		return platformCleanupResult{ProcessRunning: running}, err
	}
	return forceCleanupSingboxState(statePath, singboxOpenWrtNFTInclude, func(name string, args ...string) ([]byte, error) {
		if err := verifyPlatformSingboxStopped(); err != nil {
			return nil, err
		}
		path, err := exec.LookPath(name)
		if err != nil {
			return nil, err
		}
		timeout := linuxNetworkCommandTimeout
		if name == "fw4" {
			timeout = linuxFW4CommandTimeout
		}
		output, err := runLinuxNetworkCommand(timeout, path, args...)
		if err != nil {
			return output, cleanupCommandError(name+" "+strings.Join(args, " "), output, err)
		}
		return output, nil
	})
}

func stopPlatformSingboxForRepair() error {
	return stopPlatformSingboxForStart()
}
