package service

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMaskedAutoRedirectCleanupCommands(t *testing.T) {
	for _, family := range []struct {
		name   string
		ipv6   bool
		prefix string
	}{
		{name: "IPv4"},
		{name: "IPv6", ipv6: true, prefix: "-6 "},
	} {
		t.Run(family.name, func(t *testing.T) {
			dir := t.TempDir()
			ipPath := filepath.Join(dir, "ip")
			logPath := filepath.Join(dir, "commands")
			t.Setenv("ACKWRAP_TEST_IP_COMMANDS", logPath)
			if err := os.WriteFile(ipPath, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$ACKWRAP_TEST_IP_COMMANDS\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			snapshot := parseIPRuleSnapshot(maskedAutoRedirectRules)
			if errs := cleanupIPRuleSnapshot(ipPath, family.ipv6, snapshot, nil, nil, snapshot.managedRuleIDs()); len(errs) != 0 {
				t.Fatalf("masked rule cleanup failed: %v", errs)
			}
			commands, err := os.ReadFile(logPath)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{
				family.prefix + "rule del priority 9002 from all type nop",
				family.prefix + "rule del priority 9000 from all fwmark 0x2024/0x2027 goto 9002",
				family.prefix + "rule del priority 9001 from all fwmark 0x2023/0x2027 lookup 2022",
				family.prefix + "rule del priority 32768 not from all fwmark 0x2024/0x2027 lookup 2022",
			}
			if got := strings.Split(strings.TrimSpace(string(commands)), "\n"); !reflect.DeepEqual(got, want) {
				t.Fatalf("cleanup commands = %q, want %q", got, want)
			}
		})
	}
}
