package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

type installerUpdateFixture struct {
	svc      *InstallerService
	library  string
	original []byte
	running  bool
	calls    []string
}

func newInstallerUpdateFixture(t *testing.T) *installerUpdateFixture {
	t.Helper()
	root := t.TempDir()
	name, library := "sing-box", "libcronet.so"
	if runtime.GOOS == "windows" {
		name, library = "sing-box.exe", "libcronet.dll"
	} else if runtime.GOOS == "darwin" {
		library = "libcronet.dylib"
	}
	p := &paths.Paths{DataDir: root, BinaryDir: filepath.Join(root, "bin"), ConfigDir: filepath.Join(root, "config"), DownloadsDir: filepath.Join(root, "downloads")}
	p.BinaryPath, p.ConfigPath = filepath.Join(p.BinaryDir, name), filepath.Join(p.ConfigDir, "config.json")
	for _, dir := range []string{p.BinaryDir, p.ConfigDir, p.DownloadsDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open(filepath.Join(root, "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &installerUpdateFixture{
		svc: NewInstallerService(db, p, NewRealtimeService()), library: library, running: true,
		original: []byte(`{"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],"route":{"rules":[]},"experimental":{"cache_file":{"enabled":false,"buffer_size":"1 MiB","flush_interval":"1s"}},"endpoints":[{"type":"wireguard","on_demand":false}]}`),
	}
	for name, data := range map[string][]byte{p.BinaryPath: []byte("old binary"), filepath.Join(p.BinaryDir, library): []byte("old library"), p.ConfigPath: f.original} {
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	f.svc.coreRunning = func() bool { return f.running }
	f.svc.stopCore = func() error { f.calls = append(f.calls, "stop"); f.running = false; return nil }
	f.svc.startCore = func() error {
		data, _ := os.ReadFile(p.BinaryPath)
		if string(data) == "new binary" {
			f.calls = append(f.calls, "start-new")
		} else {
			f.calls = append(f.calls, "start-old")
		}
		f.running = true
		return nil
	}
	f.svc.candidateValidator = func(binary, config, version string) error {
		f.calls = append(f.calls, "validate")
		if version != "1.14.3" || filepath.Dir(binary) == p.BinaryDir {
			t.Fatal("preflight did not use the staged executable")
		}
		data, err := os.ReadFile(config)
		if err != nil || strings.Contains(string(data), "on_demand") || strings.Contains(string(data), "buffer_size") {
			t.Fatal("preflight did not use the migrated config")
		}
		assertUpdateFile(t, p.ConfigPath, string(f.original))
		assertUpdateFile(t, p.BinaryPath, "old binary")
		return nil
	}
	url, err := f.svc.buildDownloadURL("1.14.3")
	if err != nil {
		t.Fatal(err)
	}
	asset := filepath.Base(url)
	archive := filepath.Join(p.DownloadsDir, asset)
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		writer := zip.NewWriter(file)
		for _, entry := range []struct{ name, content string }{{name, "new binary"}, {library, "new library"}} {
			out, err := writer.Create("release/" + entry.name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := out.Write([]byte(entry.content)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(file)
		writer := tar.NewWriter(gz)
		writeTarEntry(t, writer, "release/"+name, "new binary", 0755)
		writeTarEntry(t, writer, "release/"+library, "new library", 0644)
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	f.svc.latest = &singboxRelease{Version: "1.14.3", Assets: []singboxReleaseAsset{{Name: asset, Digest: testSHA256Digest(data), Size: int64(len(data))}}}
	f.svc.latestAt = time.Now()
	return f
}

func assertUpdateFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("unexpected content of %s: error=%v", filepath.Base(path), err)
	}
}

func (f *installerUpdateFixture) assertRestored(t *testing.T, running bool) {
	t.Helper()
	assertUpdateFile(t, f.svc.paths.BinaryPath, "old binary")
	assertUpdateFile(t, filepath.Join(f.svc.paths.BinaryDir, f.library), "old library")
	assertUpdateFile(t, f.svc.paths.ConfigPath, string(f.original))
	if f.running != running {
		t.Fatalf("running=%t, want %t", f.running, running)
	}
	state, err := f.svc.store.GetInstallState()
	if err != nil || state.Status != model.InstallFailed || state.Version == "1.14.3" || state.Error == "" {
		t.Fatalf("failed update status: %+v error=%v", state, err)
	}
}

func TestInstallerUpdatePreflightFailureKeepsRunningCore(t *testing.T) {
	f := newInstallerUpdateFixture(t)
	f.svc.candidateValidator = func(string, string, string) error { return errors.New("injected preflight failure") }
	f.svc.runInstall()
	f.assertRestored(t, true)
	if len(f.calls) != 0 {
		t.Fatalf("core touched before validation: %v", f.calls)
	}
}

func TestInstallerUpdateBlocksUnsupportedStackBeforeStopping(t *testing.T) {
	f := newInstallerUpdateFixture(t)
	f.original = []byte(`{"inbounds":[{"type":"tun","stack":"go"}]}`)
	if err := os.WriteFile(f.svc.paths.ConfigPath, f.original, 0600); err != nil {
		t.Fatal(err)
	}
	f.svc.runInstall()
	f.assertRestored(t, true)
	state, _ := f.svc.store.GetInstallState()
	if len(f.calls) != 0 || !strings.Contains(state.Error, "inbounds[0].stack") {
		t.Fatal("unsupported runtime stack was not blocked before stopping")
	}
}

func TestInstallerUpdateReplacementFailureRestartsPreviousCore(t *testing.T) {
	f := newInstallerUpdateFixture(t)
	f.svc.replaceFile = func(source, target string) error {
		if filepath.Base(filepath.Dir(source)) == "files" && filepath.Base(source) == f.library {
			return errors.New("injected library failure")
		}
		return atomicReplaceFile(source, target)
	}
	f.svc.runInstall()
	f.assertRestored(t, true)
	if strings.Join(f.calls, ",") != "validate,stop,start-old" {
		t.Fatalf("lifecycle: %v", f.calls)
	}
}

func TestInstallerUpdateRestartFailureRestoresConfigLibrariesAndPreferences(t *testing.T) {
	f := newInstallerUpdateFixture(t)
	if _, err := f.svc.store.MigrateDNSIndependentCache(true); err != nil {
		t.Fatal(err)
	}
	f.svc.startCore = func() error {
		data, _ := os.ReadFile(f.svc.paths.BinaryPath)
		if string(data) == "new binary" {
			f.calls = append(f.calls, "start-new")
			config, _ := os.ReadFile(f.svc.paths.ConfigPath)
			if strings.Contains(string(config), "on_demand") {
				t.Fatal("new core started without migrated config")
			}
			f.running = true // A failed readiness hook may still leave a process alive.
			return errors.New("injected readiness failure")
		}
		f.calls = append(f.calls, "start-old")
		assertUpdateFile(t, f.svc.paths.ConfigPath, string(f.original))
		state, _ := f.svc.store.GetInstallState()
		if state.Version == "1.14.3" {
			t.Fatal("old core restart reported the new version")
		}
		f.running = true
		return nil
	}
	f.svc.runInstall()
	f.assertRestored(t, true)
	settings, err := f.svc.store.GetDNSGlobalSettings()
	if err != nil || !settings.IndependentCache {
		t.Fatalf("failed update changed DNS preference: %v", err)
	}
	if strings.Join(f.calls, ",") != "validate,stop,start-new,stop,start-old" {
		t.Fatalf("lifecycle: %v", f.calls)
	}
}

func TestInstallerUpdatePreservesBackupsOnIncompleteRollback(t *testing.T) {
	for _, failStop := range []bool{false, true} {
		t.Run(map[bool]string{true: "stop-failure", false: "file-rollback-failure"}[failStop], func(t *testing.T) {
			f := newInstallerUpdateFixture(t)
			f.svc.startCore = func() error {
				f.running = failStop
				f.calls = append(f.calls, "start-new")
				return errors.New("injected startup failure")
			}
			if failStop {
				f.svc.stopCore = func() error {
					f.calls = append(f.calls, "stop")
					if len(f.calls) > 3 {
						return errors.New("injected stop failure")
					}
					f.running = false
					return nil
				}
			} else {
				f.svc.replaceFile = func(source, target string) error {
					if filepath.Base(filepath.Dir(source)) == "backups" {
						return errors.New("injected rollback failure")
					}
					return atomicReplaceFile(source, target)
				}
			}
			f.svc.runInstall()
			state, _ := f.svc.store.GetInstallState()
			if state.Status != model.InstallFailed || !strings.Contains(state.Error, "recovery files preserved") {
				t.Fatal("missing failed recovery status")
			}
			staging, _ := filepath.Glob(filepath.Join(f.svc.paths.BinaryDir, ".ackwrap-runtime-*"))
			if len(staging) != 1 {
				t.Fatal("recovery backups were deleted")
			}
			assertUpdateFile(t, filepath.Join(staging[0], "backups", filepath.Base(f.svc.paths.BinaryPath)), "old binary")
			assertUpdateFile(t, filepath.Join(staging[0], "backups", "active-config.json"), string(f.original))
			if strings.Contains(strings.Join(f.calls, ","), "start-old") {
				t.Fatal("unsafe previous core restart")
			}
		})
	}
}

func TestInstallerUpdateSuccessAndStoppedCore(t *testing.T) {
	for _, running := range []bool{true, false} {
		t.Run(map[bool]string{true: "running", false: "stopped"}[running], func(t *testing.T) {
			f := newInstallerUpdateFixture(t)
			f.running = running
			f.svc.runInstall()
			assertUpdateFile(t, f.svc.paths.BinaryPath, "new binary")
			assertUpdateFile(t, filepath.Join(f.svc.paths.BinaryDir, f.library), "new library")
			data, _ := os.ReadFile(f.svc.paths.ConfigPath)
			if strings.Contains(string(data), "on_demand") || f.running != running {
				t.Fatal("successful update did not preserve lifecycle or migrate config")
			}
			state, err := f.svc.store.GetInstallState()
			if err != nil || state.Status != model.InstallDone || state.Version != "1.14.3" || state.Error != "" {
				t.Fatalf("success status: %+v error=%v", state, err)
			}
			staging, _ := filepath.Glob(filepath.Join(f.svc.paths.BinaryDir, ".ackwrap-runtime-*"))
			if len(staging) != 0 {
				t.Fatal("completed update retained transient runtime files")
			}
		})
	}
}

func TestInstallerUpdateDNSMigrationFailureRollsBack(t *testing.T) {
	f := newInstallerUpdateFixture(t)
	if _, err := f.svc.store.MigrateDNSIndependentCache(true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.store.DB().Exec(`CREATE TRIGGER reject_core_dns BEFORE DELETE ON app_settings
		WHEN OLD.key IN ('dns.independent_cache', 'dns_global.independent_cache')
		BEGIN SELECT RAISE(ABORT, 'injected DNS migration failure'); END`); err != nil {
		t.Fatal(err)
	}
	f.svc.runInstall()
	f.assertRestored(t, true)
	settings, err := f.svc.store.GetDNSGlobalSettings()
	if err != nil || !settings.IndependentCache {
		t.Fatalf("failed atomic DB migration changed DNS preference: %v", err)
	}
	if strings.Join(f.calls, ",") != "validate,stop,start-new,stop,start-old" {
		t.Fatalf("lifecycle: %v", f.calls)
	}
}

func TestInstallerUpdateBlocksConcurrentLifecycleAndConfigWriters(t *testing.T) {
	f := newInstallerUpdateFixture(t)
	f.svc.coreService = &SingboxService{}
	validated := make(chan struct{})
	continueUpdate := make(chan struct{})
	f.svc.candidateValidator = func(string, string, string) error {
		close(validated)
		<-continueUpdate
		return nil
	}
	updated := make(chan struct{})
	go func() { f.svc.runInstall(); close(updated) }()
	<-validated
	if f.svc.coreService.lifecycleMu.TryLock() {
		f.svc.coreService.lifecycleMu.Unlock()
		t.Error("update did not block concurrent core lifecycle actions")
	}
	if configFileMu.TryLock() {
		configFileMu.Unlock()
		t.Error("update did not block active configuration changes")
	}
	writerStarted, writerFinished := make(chan struct{}), make(chan struct{})
	go func() {
		close(writerStarted)
		release := f.svc.store.HoldConfigUpdate()
		release()
		close(writerFinished)
	}()
	<-writerStarted
	select {
	case <-writerFinished:
		t.Error("configuration-visible writer was not blocked")
	case <-time.After(30 * time.Millisecond):
	}
	close(continueUpdate)
	<-updated
	<-writerFinished
}
