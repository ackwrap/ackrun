package service

import "testing"

func TestUnsupportedNodeTypeUsesSubscriptionProtocolMatrix(t *testing.T) {
	for _, protocol := range []string{"ssh", "shadowsocksr", "hy2", "wg"} {
		if isUnsupportedNodeType(protocol) {
			t.Fatalf("expected %s to be supported", protocol)
		}
	}
	for _, protocol := range []string{"shadowtls", "tor", "bridge", "openvpn-client", "mieru", "unknown"} {
		if !isUnsupportedNodeType(protocol) {
			t.Fatalf("expected %s to be unsupported", protocol)
		}
	}
}
