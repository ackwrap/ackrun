package service

// realIPPingBypassRule runs after terminating device policies. On Linux,
// auto_redirect pre-matching turns bypass into the existing firewall output mark
// so echo requests use the system route without entering the TUN stack.
func (s *ConfigGeneratorService) realIPPingBypassRule() (map[string]interface{}, error) {
	fakeIPs, err := s.deviceFakeIPCIDRs()
	if err != nil {
		return nil, err
	}
	// A positive IP match also excludes unresolved domains. Include configured,
	// active and historical FakeIP pools so virtual destinations never escape.
	realIPs, err := excludeDeviceFakeIPCIDRs([]string{"0.0.0.0/0", "::/0"}, fakeIPs)
	if err != nil {
		return nil, err
	}
	if len(realIPs) == 0 {
		return nil, nil
	}
	return map[string]interface{}{
		"inbound":  []string{"tun-in"},
		"network":  []string{"icmp"},
		"ip_cidr":  realIPs,
		"action":   "bypass",
		"outbound": "direct",
	}, nil
}
