package service

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestParseSSHMonitorOutput(t *testing.T) {
	collectedAt := time.UnixMilli(1724500000000)
	output := []byte("ACKWRAP_MONITOR_V1\n" +
		"HOST\tvm-01\n" +
		"USER\toperator\n" +
		"CPU\t12000\t9000\n" +
		"MEM\t4294967296\t1073741824\n" +
		"NET\t123456\t654321\n" +
		"UPTIME\t3600\n" +
		"LOGINS\t2\n" +
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
	if snapshot.CPUTotal != 12000 || snapshot.CPUIdle != 9000 {
		t.Fatalf("unexpected CPU counters: %#v", snapshot)
	}
	if snapshot.MemoryTotalBytes != 4294967296 || snapshot.MemoryAvailableBytes != 1073741824 {
		t.Fatalf("unexpected memory counters: %#v", snapshot)
	}
	if snapshot.NetworkReceivedBytes != 123456 || snapshot.NetworkTransmittedBytes != 654321 {
		t.Fatalf("unexpected network counters: %#v", snapshot)
	}
	if snapshot.UptimeSeconds != 3600 || snapshot.LoginSessions != 2 || snapshot.CollectedAt != collectedAt.UnixMilli() {
		t.Fatalf("unexpected monitor metadata: %#v", snapshot)
	}
	if len(snapshot.Disks) != 2 || snapshot.Disks[0].MountPoint != "/" || snapshot.Disks[0].UsagePercent != 20 {
		t.Fatalf("unexpected disk data: %#v", snapshot.Disks)
	}
	if snapshot.Disks[1].MountPoint != "/mnt/data disk" || snapshot.Disks[1].UsagePercent != 50 {
		t.Fatalf("unexpected spaced disk mount: %#v", snapshot.Disks[1])
	}
}

func TestParseSSHDeviceDetailsOutput(t *testing.T) {
	collectedAt := time.UnixMilli(1724500000000)
	output := []byte("ACKWRAP_DETAILS_V1\n" +
		"HOST\tvm-01\n" +
		"OS\tDebian GNU/Linux 13\n" +
		"KERNEL\t6.12.0-amd64\n" +
		"ARCH\tx86_64\n" +
		"CPU_MODEL\tExample CPU\n" +
		"CPU_CORES\t4\n" +
		"MEM\t4294967296\t1073741824\t2147483648\t1073741824\n" +
		"UPTIME\t3600\n" +
		"LOAD\t0.25\t0.50\t0.75\n" +
		"VIRT\tkvm\n" +
		"PACKAGE_MANAGER\tapt-get\n" +
		"DISK\t/dev/root\t10737418240\t2147483648\t8589934592\t/\n" +
		"DISK\t/dev/root\t10737418240\t2147483648\t8589934592\t/\n" +
		"DISK\t-\t21474836480\t10737418240\t10737418240\t/mnt/data disk\n" +
		"SOFTWARE\tdocker\t1\tDocker version 28.0.0\n" +
		"SOFTWARE\tdocker-compose\t0\t\n" +
		"SOFTWARE\tsing-box\t1\tsing-box version 1.13.14\n" +
		"END\n")
	details, err := parseSSHDeviceDetailsOutput(output, collectedAt)
	if err != nil {
		t.Fatal(err)
	}
	if details.CPUModel != "Example CPU" || details.CPUCores != 4 {
		t.Fatalf("unexpected CPU details: %#v", details)
	}
	if details.Hostname != "vm-01" || details.OSName != "Debian GNU/Linux 13" || details.KernelVersion != "6.12.0-amd64" || details.Architecture != "x86_64" {
		t.Fatalf("unexpected system details: %#v", details)
	}
	if details.MemoryTotalBytes != 4294967296 || details.MemoryAvailableBytes != 1073741824 {
		t.Fatalf("unexpected memory details: %#v", details)
	}
	if details.SwapTotalBytes != 2147483648 || details.SwapAvailableBytes != 1073741824 {
		t.Fatalf("unexpected swap details: %#v", details)
	}
	if details.UptimeSeconds != 3600 || details.CollectedAt != collectedAt.UnixMilli() {
		t.Fatalf("unexpected details metadata: %#v", details)
	}
	if details.LoadAverage1 != 0.25 || details.LoadAverage5 != 0.5 || details.LoadAverage15 != 0.75 {
		t.Fatalf("unexpected load averages: %#v", details)
	}
	if details.Virtualization != "kvm" || details.PackageManager != "apt-get" {
		t.Fatalf("unexpected platform details: %#v", details)
	}
	if len(details.Disks) != 2 || details.Disks[0].MountPoint != "/" || details.Disks[0].UsagePercent != 20 {
		t.Fatalf("unexpected disk data: %#v", details.Disks)
	}
	if details.Disks[1].MountPoint != "/mnt/data disk" || details.Disks[1].UsagePercent != 50 {
		t.Fatalf("unexpected spaced disk mount: %#v", details.Disks[1])
	}
	if len(details.Software) != 3 || !details.Software[0].Installed || details.Software[1].Installed || !details.Software[2].Installed {
		t.Fatalf("unexpected software status: %#v", details.Software)
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
	if strings.Contains(sshMonitorCommand, "docker") {
		t.Fatal("lightweight monitor command includes static device checks")
	}
	for _, required := range []string{"hostname", "df -Pk"} {
		if !strings.Contains(sshMonitorCommand, required) {
			t.Fatalf("lightweight monitor command is missing required probe %q", required)
		}
	}
	if !strings.Contains(sshDeviceDetailsCommand, "docker") || !strings.Contains(sshDeviceDetailsCommand, "sing-box") {
		t.Fatal("device details command is missing software checks")
	}
	for _, required := range []string{"hostname", "uname", "df", "systemd-detect-virt", "apt-get", "dnf", "yum", "apk", "opkg", "pacman"} {
		pattern := regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(required) + `([^A-Za-z0-9_-]|$)`)
		if !pattern.MatchString(sshDeviceDetailsCommand) {
			t.Fatalf("device details command is missing restored probe %q", required)
		}
	}
	if !strings.Contains(sshDeviceDetailsCommand, "os-release") {
		t.Fatal("device details command is missing operating system metadata")
	}
}

func TestSSHDeviceDetailsJSONIncludesRestoredFields(t *testing.T) {
	encoded, err := json.Marshal(&model.SSHDeviceDetails{})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"hostname": true, "os_name": true,
		"kernel_version": true, "architecture": true,
		"cpu_model": true, "cpu_cores": true,
		"memory_total_bytes": true, "memory_available_bytes": true,
		"swap_total_bytes": true, "swap_available_bytes": true,
		"uptime_seconds": true, "load_average_1": true,
		"load_average_5": true, "load_average_15": true,
		"virtualization": true, "package_manager": true,
		"disks":    true,
		"software": true, "collected_at": true,
	}
	if len(fields) != len(allowed) {
		t.Fatalf("unexpected device details JSON fields: %s", encoded)
	}
	for field := range fields {
		if !allowed[field] {
			t.Fatalf("device details JSON includes disallowed field %q", field)
		}
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

func TestParseSSHDeviceDetailsOutputRejectsIncompleteData(t *testing.T) {
	for name, output := range map[string]string{
		"missing header": "CPU_CORES\t1\nEND\n",
		"missing end":    "ACKWRAP_DETAILS_V1\nCPU_CORES\t1\n",
		"invalid memory": "ACKWRAP_DETAILS_V1\nCPU_CORES\t1\nMEM\t1\t2\t0\t0\nUPTIME\t1\nLOAD\t0\t0\t0\nEND\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSSHDeviceDetailsOutput([]byte(output), time.Now()); err == nil {
				t.Fatal("expected invalid device details output to be rejected")
			}
		})
	}
}
