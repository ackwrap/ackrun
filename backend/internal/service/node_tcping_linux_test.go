//go:build linux

package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveTCPingServerRejectsLoopbackOnlyDNS(t *testing.T) {
	resolverFile := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(resolverFile, []byte("nameserver 127.0.0.1\nnameserver ::1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	originalFiles := tcpingResolverFiles
	tcpingResolverFiles = []string{resolverFile}
	t.Cleanup(func() { tcpingResolverFiles = originalFiles })

	_, err := resolveTCPingServer(context.Background(), "example.com")
	if err == nil || !strings.Contains(err.Error(), "非回环系统 DNS") {
		t.Fatalf("resolve error = %v", err)
	}
}
