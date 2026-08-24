package service

import (
	"strings"
	"testing"
	"time"
)

func TestParseSSHMonitorOutput(t *testing.T) {
	collectedAt := time.UnixMilli(1724500000000)
	output := []byte("ACKWRAP_MONITOR_V1\n" +
		"HOST\tvm-01\n" +
		"USER\toperator\n" +
		"OS\tDebian GNU/Linux 13\n" +
		"KERNEL\t6.12.0-amd64\n" +
		"ARCH\tx86_64\n" +
		"CPU_MODEL\tExample CPU\n" +
		"CPU_CORES\t4\n" +
		"CPU\t12000\t9000\n" +
		"MEM\t4294967296\t1073741824\t2147483648\t1073741824\n" +
		"NET\t123456\t654321\n" +
		"UPTIME\t3600\n" +
		"LOAD\t0.25\t0.50\t0.75\n" +
		"LOGINS\t2\n" +
		"VIRT\tkvm\n" +
		"PACKAGE_MANAGER\tapt-get\n" +
		"SOFTWARE\tdocker\t1\tDocker version 28.0.0\n" +
		"SOFTWARE\tdocker-compose\t0\t\n" +
		"DISK\t/dev/root\t10737418240\t2147483648\t8589934592\t/\n" +
		"DISK\t/dev/root\t10737418240\t2147483648\t8589934592\t/\n" +
		"DISK\t-\t21474836480\t10737418240\t10737418240\t/mnt/data disk\n" +
		"END\n")
	snapshot, err := parseSSHMonitorOutput(output, collectedAt)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Hostname != "vm-01" || snapshot.Username != "operator" {
		t.Fatalf("unexpected host identity: %#v", snapshot)
	}
	if snapshot.OSName != "Debian GNU/Linux 13" || snapshot.KernelVersion != "6.12.0-amd64" || snapshot.Architecture != "x86_64" {
		t.Fatalf("unexpected system identity: %#v", snapshot)
	}
	if snapshot.CPUModel != "Example CPU" || snapshot.CPUCores != 4 {
		t.Fatalf("unexpected CPU details: %#v", snapshot)
	}
	if snapshot.CPUTotal != 12000 || snapshot.CPUIdle != 9000 {
		t.Fatalf("unexpected CPU counters: %#v", snapshot)
	}
	if snapshot.MemoryTotalBytes != 4294967296 || snapshot.MemoryAvailableBytes != 1073741824 {
		t.Fatalf("unexpected memory counters: %#v", snapshot)
	}
	if snapshot.SwapTotalBytes != 2147483648 || snapshot.SwapAvailableBytes != 1073741824 {
		t.Fatalf("unexpected swap counters: %#v", snapshot)
	}
	if snapshot.NetworkReceivedBytes != 123456 || snapshot.NetworkTransmittedBytes != 654321 {
		t.Fatalf("unexpected network counters: %#v", snapshot)
	}
	if snapshot.UptimeSeconds != 3600 || snapshot.LoginSessions != 2 || snapshot.CollectedAt != collectedAt.UnixMilli() {
		t.Fatalf("unexpected monitor metadata: %#v", snapshot)
	}
	if snapshot.LoadAverage1 != 0.25 || snapshot.LoadAverage5 != 0.5 || snapshot.LoadAverage15 != 0.75 {
		t.Fatalf("unexpected load averages: %#v", snapshot)
	}
	if snapshot.Virtualization != "kvm" || snapshot.PackageManager != "apt-get" {
		t.Fatalf("unexpected platform details: %#v", snapshot)
	}
	if len(snapshot.Software) != 2 || !snapshot.Software[0].Installed || snapshot.Software[1].Installed {
		t.Fatalf("unexpected software status: %#v", snapshot.Software)
	}
	if len(snapshot.Disks) != 2 || snapshot.Disks[0].MountPoint != "/" || snapshot.Disks[0].UsagePercent != 20 {
		t.Fatalf("unexpected disk data: %#v", snapshot.Disks)
	}
	if snapshot.Disks[1].MountPoint != "/mnt/data disk" || snapshot.Disks[1].UsagePercent != 50 {
		t.Fatalf("unexpected spaced disk mount: %#v", snapshot.Disks[1])
	}
}

func TestSSHMonitorOutputCapsBufferedContent(t *testing.T) {
	output := &sshMonitorOutput{}
	content := make([]byte, sshMonitorMaximumOutput+1024)
	written, err := output.Write(content)
	if err != nil || written != len(content) {
		t.Fatalf("unexpected capped writer result: written=%d err=%v", written, err)
	}
	if !output.overflow || output.buffer.Len() != sshMonitorMaximumOutput {
		t.Fatalf("monitor output was not capped: overflow=%t size=%d", output.overflow, output.buffer.Len())
	}
}

func TestSSHMonitorCommandKeepsStaticChecksOutOfPolling(t *testing.T) {
	if strings.Contains(sshMonitorCommand, "docker") || strings.Contains(sshMonitorCommand, "systemd-detect-virt") {
		t.Fatal("lightweight monitor command includes static device checks")
	}
	if !strings.Contains(sshMonitorDetailsCommand, "docker") || !strings.Contains(sshMonitorDetailsCommand, "systemd-detect-virt") {
		t.Fatal("device details command is missing static checks")
	}
}

func TestMonitorFloatRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		t.Run(value, func(t *testing.T) {
			if _, err := monitorFloat(value); err == nil {
				t.Fatalf("expected %q to be rejected", value)
			}
		})
	}
}

func TestParseSSHMonitorOutputRejectsIncompleteData(t *testing.T) {
	for name, output := range map[string]string{
		"missing header": "CPU\t1\t1\nEND\n",
		"missing end":    "ACKWRAP_MONITOR_V1\nCPU\t1\t1\n",
		"invalid memory": "ACKWRAP_MONITOR_V1\nCPU\t2\t1\nMEM\t1\t2\nNET\t0\t0\nUPTIME\t1\nEND\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSSHMonitorOutput([]byte(output), time.Now()); err == nil {
				t.Fatal("expected invalid monitor output to be rejected")
			}
		})
	}
}
