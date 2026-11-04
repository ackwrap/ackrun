package service

import (
	"net/netip"
	"slices"
	"strings"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

func (svc *SSHHostService) PrivacySettings() (*model.SSHPrivacySettings, error) {
	return svc.store.GetSSHPrivacySettings()
}

func (svc *SSHHostService) UpdatePrivacySettings(settings model.SSHPrivacySettings) (*model.SSHPrivacySettings, error) {
	if err := svc.store.SetSSHPrivacySettings(settings); err != nil {
		logging.Error("ssh_host.privacy", "更新 SSH 地址隐藏设置失败")
		return nil, err
	}
	logging.Info("ssh_host.privacy", "更新 SSH 地址隐藏设置: hide_host_address=%t", settings.HideHostAddress)
	return &settings, nil
}

// Presentation is separate from the stored host used for SSH connections.
func (svc *SSHHostService) presentHosts(items []model.SSHHost) ([]model.SSHHost, error) {
	settings, err := svc.PrivacySettings()
	if err != nil {
		return nil, err
	}
	if settings.HideHostAddress {
		for index := range items {
			items[index] = maskSSHHost(items[index])
		}
	}
	return items, nil
}

func maskSSHHost(item model.SSHHost) model.SSHHost {
	address := item.Host
	item.Host = maskSSHAddress(address)
	item.HostHidden = true
	// Saved labels and old connection errors can also contain the address.
	for _, text := range []*string{&item.Name, &item.GroupName, &item.Username, &item.CredentialName, &item.NodeExposureName, &item.Notes, &item.LastErrorMessage} {
		if address != "" {
			*text = strings.ReplaceAll(*text, address, item.Host)
		}
	}
	item.Tags = append([]string{}, item.Tags...)
	for index, tag := range item.Tags {
		if address != "" {
			item.Tags[index] = strings.ReplaceAll(tag, address, item.Host)
		}
	}
	return item
}

func (svc *SSHHostService) presentHost(item *model.SSHHost) (*model.SSHHost, error) {
	items, err := svc.presentHosts([]model.SSHHost{*item})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (svc *SSHHostService) restoreHiddenHostFields(request *model.SSHHostRequest, existing *model.SSHHost) error {
	if request.KeepHostAddress {
		if strings.TrimSpace(request.Host) != "" {
			return sshError("SSH_HOST_INVALID", "保留原地址时请勿同时填写新地址", nil)
		}
		request.Host = existing.Host
	}
	settings, err := svc.PrivacySettings()
	if err != nil {
		return err
	}
	if !request.KeepHostAddress && !settings.HideHostAddress {
		return nil
	}
	visible := maskSSHHost(*existing)
	// Preserve the address even when a user edits text around its placeholder.
	for _, field := range []struct {
		input        *string
		shown, saved string
	}{
		{&request.Name, visible.Name, existing.Name},
		{&request.GroupName, visible.GroupName, existing.GroupName},
		{&request.Username, visible.Username, existing.Username},
		{&request.Notes, visible.Notes, existing.Notes},
	} {
		if field.shown != field.saved {
			*field.input = strings.ReplaceAll(*field.input, visible.Host, existing.Host)
		}
	}
	if slices.Equal(request.Tags, visible.Tags) {
		request.Tags = existing.Tags
	} else if !slices.Equal(visible.Tags, existing.Tags) {
		request.Tags = append([]string{}, request.Tags...)
		for index, tag := range request.Tags {
			request.Tags[index] = strings.ReplaceAll(tag, visible.Host, existing.Host)
		}
	}
	return nil
}

func maskSSHAddress(host string) string {
	if address, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		address = address.Unmap()
		if address.Is4() {
			octets := strings.Split(address.String(), ".")
			return strings.Join(octets[:2], ".") + ".*.*"
		}
		// Expanding the first groups avoids revealing short/compressed IPv6 hosts.
		groups := strings.Split(address.StringExpanded(), ":")
		return groups[0] + ":" + groups[1] + ":*"
	}
	// Domain names can identify a host just as directly as an IP address.
	return "***"
}
