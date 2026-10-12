package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

// Opt-in, synthetic-only test against a downloaded release binary and its
// sibling runtime libraries. It never touches an installed core or TUN device.
func TestInstallerUpdateWithReleaseCore(t *testing.T) {
	binary := os.Getenv("ACKWRAP_SINGBOX_114_BIN")
	if binary == "" {
		t.Skip("set ACKWRAP_SINGBOX_114_BIN to test the actual 1.14.3 release core")
	}
	if readSingboxVersion(binary) != "1.14.3" {
		t.Fatal("release-core update test requires version 1.14.3")
	}
	for _, failRestart := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "readiness-rollback"}[failRestart], func(t *testing.T) {
			root := t.TempDir()
			p := &paths.Paths{DataDir: root, BinaryDir: filepath.Join(root, "bin"), ConfigDir: filepath.Join(root, "config")}
			p.BinaryPath, p.ConfigPath = filepath.Join(p.BinaryDir, filepath.Base(binary)), filepath.Join(p.ConfigDir, "config.json")
			for _, dir := range []string{p.BinaryDir, p.ConfigDir} {
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			db, err := store.Open(filepath.Join(root, "synthetic.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := copyFile(binary, p.BinaryPath); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p.BinaryPath, 0755); err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := listener.Addr().(*net.TCPAddr).Port
			listener.Close()
			original := []byte(fmt.Sprintf(`{"log":{"level":"error"},"outbounds":[{"type":"direct","tag":"direct"}],"services":[{"type":"api","listen":"127.0.0.1","listen_port":%d,"secret":"synthetic-only"}]}`, port))
			// Managed-config migration adds the missing proxy group and route. Both
			// versions remain valid, allowing a real restart with the original bytes.
			ready, _, err := migrateCoreCompatibilityData(original, "1.14.3")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p.ConfigPath, original, 0600); err != nil {
				t.Fatal(err)
			}
			rt := NewRealtimeService()
			core := NewSingboxService(p, rt, NewCoreLogService(), db)
			client := NewNodeExposureRuntimeClient("synthetic-only")
			client.baseURL = fmt.Sprintf("http://127.0.0.1:%d/api/v1", port)
			core.SetStartHooks(nil, client.WaitReady, nil)
			if _, err := core.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if core.IsRunning() {
					_, _ = core.Stop()
				}
			}()
			installer := NewInstallerService(db, p, rt)
			installer.SetCoreUpdateService(core)
			staging, err := installer.createRuntimeStagingDir()
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(staging)
			file, err := os.Open(binary)
			if err != nil {
				t.Fatal(err)
			}
			staged, err := stageRuntimeFile(staging, filepath.Base(binary), file, 0755)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			files := []stagedRuntimeFile{staged}
			library := "libcronet.so"
			if runtime.GOOS == "windows" {
				library = "libcronet.dll"
			} else if runtime.GOOS == "darwin" {
				library = "libcronet.dylib"
			}
			if file, err := os.Open(filepath.Join(filepath.Dir(binary), library)); err == nil {
				staged, err := stageRuntimeFile(staging, library, file, 0644)
				file.Close()
				if err != nil {
					t.Fatal(err)
				}
				files = append(files, staged)
			}
			attempts := 0
			core.SetStartHooks(nil, client.WaitReady, func() error {
				attempts++
				if failRestart && attempts == 1 {
					return errors.New("injected runtime synchronization failure")
				}
				return nil
			})
			err = installer.installStagedUpdate(staging, files, "1.14.3")
			if failRestart {
				if err == nil || !strings.Contains(err.Error(), "injected runtime synchronization failure") || attempts != 2 {
					t.Fatalf("real restart rollback: attempts=%d error=%v", attempts, err)
				}
				assertUpdateFile(t, p.ConfigPath, string(original))
			} else if err != nil || attempts != 1 {
				t.Fatalf("real update: attempts=%d error=%v", attempts, err)
			} else {
				assertUpdateFile(t, p.ConfigPath, string(ready))
			}
			if !core.IsRunning() {
				t.Fatal("update did not restore the running core")
			}
			if err := validateCandidateCore(p.BinaryPath, "", "9.9.9"); err == nil {
				t.Fatal("candidate version mismatch was accepted")
			}
			inactive := []byte(`{"inbounds":[{"type":"tun","stack":"system","multi_queue":false,"auto_redirect_tproxy_mark":"0x0"}],"outbounds":[{"type":"direct","tag":"direct"}],"experimental":{"cache_file":{"enabled":false,"buffer_size":"1 MiB","flush_interval":"1s"}}}`)
			inactive, _, err = migrateDevelopmentConfigData(inactive, "1.14.3")
			if err != nil {
				t.Fatal(err)
			}
			filePath := filepath.Join(p.ConfigDir, "inactive-options.json")
			if err := os.WriteFile(filePath, inactive, 0600); err != nil {
				t.Fatal(err)
			}
			if err := validateCandidateCore(p.BinaryPath, filePath, "1.14.3"); err != nil {
				t.Fatal(err)
			}
			// sing-box check accepts this stack string, so Ackwrap must reject it
			// independently before a real TUN startup can be attempted.
			if err := os.WriteFile(p.ConfigPath, []byte(`{"inbounds":[{"type":"tun","stack":"go"}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := core.validateActiveConfig(); err == nil || !strings.Contains(err.Error(), "inbounds[0].stack") {
				t.Fatal("hidden runtime-only stack failure was not caught")
			}
		})
	}
}

func TestWireGuardInactiveOnDemandWithReleaseCore(t *testing.T) {
	binary := os.Getenv("ACKWRAP_SINGBOX_114_BIN")
	if binary == "" {
		t.Skip("set ACKWRAP_SINGBOX_114_BIN for release compatibility testing")
	}
	svc := &ConfigGeneratorService{paths: &paths.Paths{BinaryPath: binary}}
	endpoint, err := svc.generateWireGuardEndpoint(&model.Node{Type: "wireguard", RawJSON: `{"type":"wireguard","on_demand":false,"address":["10.0.0.2/32"],"private_key":"AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=","peers":[{"address":"192.0.2.1","port":51820,"public_key":"AgICAgICAgICAgICAgICAgICAgICAgICAgICAgICAgI=","allowed_ips":["0.0.0.0/0"]}]}`}, "synthetic-wg", nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]interface{}{"endpoints": []interface{}{endpoint}, "outbounds": []interface{}{map[string]interface{}{"type": "direct", "tag": "direct"}}})
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(t.TempDir(), "synthetic.json")
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateCandidateCore(binary, filePath, "1.14.3"); err != nil {
		t.Fatal(err)
	}
}

func TestCoreCheckDiagnosticRedactsConnectionValues(t *testing.T) {
	value := map[string]interface{}{"server": "192.0.2.1", "uuid": "synthetic-uuid", "tls": map[string]interface{}{"server_name": "synthetic.example.test"}, "peers": []interface{}{map[string]interface{}{"address": "192.0.2.2", "private_key": "synthetic-private"}}}
	message := redactCoreCheckValues("192.0.2.1 synthetic-uuid synthetic.example.test 192.0.2.2 synthetic-private", value)
	if strings.Contains(message, "synthetic") || strings.Contains(message, "192.0.2.") {
		t.Fatal("diagnostic contained connection values")
	}
}
