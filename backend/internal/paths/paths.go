package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const activeConfigMarkerName = ".active-config"

func IsConfigBackupName(name string) bool {
	name = strings.ToLower(name)
	return strings.HasSuffix(name, ".bak.json") ||
		(strings.HasPrefix(name, "config.backup.") && strings.HasSuffix(name, ".json"))
}

func IsConfigFileName(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".json") && !IsConfigBackupName(name)
}

type Paths struct {
	DataDir       string
	BinaryDir     string
	BinaryPath    string
	ConfigDir     string
	ConfigPath    string
	RulesDir      string
	GeoDir        string
	DownloadsDir  string
	DashboardsDir string
	DBPath        string
}

func Default() *Paths {
	var dataDir string
	if env := os.Getenv("ACKWRAP_DATA_DIR"); env != "" {
		dataDir = env
	} else {
		switch runtime.GOOS {
		case "windows":
			dataDir = filepath.Join(os.Getenv("USERPROFILE"), "ackwrap")
		case "darwin":
			home, _ := os.UserHomeDir()
			dataDir = filepath.Join(home, "ackwrap")
		default:
			dataDir = "/etc/ackwrap"
		}
	}

	var binaryDir string
	if env := os.Getenv("ACKWRAP_BINARY_DIR"); env != "" {
		binaryDir = env
	} else {
		binaryDir = filepath.Join(dataDir, "bin")
	}

	binaryName := "sing-box"
	if runtime.GOOS == "windows" {
		binaryName = "sing-box.exe"
	}

	configDir := filepath.Join(dataDir, "config")
	rulesDir := filepath.Join(dataDir, "rules")
	geoDir := filepath.Join(dataDir, "geo")
	downloadsDir := filepath.Join(dataDir, "downloads")
	dashboardsDir := filepath.Join(dataDir, "dash")

	return &Paths{
		DataDir:       dataDir,
		BinaryDir:     binaryDir,
		BinaryPath:    filepath.Join(binaryDir, binaryName),
		ConfigDir:     configDir,
		ConfigPath:    filepath.Join(configDir, "config.json"),
		RulesDir:      rulesDir,
		GeoDir:        geoDir,
		DownloadsDir:  downloadsDir,
		DashboardsDir: dashboardsDir,
		DBPath:        filepath.Join(dataDir, "ackwrap.db"),
	}
}

func (p *Paths) EnsureDirs() error {
	if p.DashboardsDir == "" {
		p.DashboardsDir = filepath.Join(p.DataDir, "dash")
	}
	dirs := []string{p.DataDir, p.BinaryDir, p.ConfigDir, p.RulesDir, p.GeoDir, p.DownloadsDir, p.DashboardsDir}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}

	legacyConfig := filepath.Join(p.DataDir, "config.json")
	if _, ok, err := p.ActiveConfigPath(); err != nil {
		return err
	} else if !ok {
		if _, err := os.Stat(legacyConfig); err == nil {
			if err := os.Rename(legacyConfig, p.ConfigPath); err != nil {
				return err
			}
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (p *Paths) ActiveConfigPath() (string, bool, error) {
	entries, err := os.ReadDir(p.ConfigDir)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}

	marker, err := os.ReadFile(p.ActiveConfigMarkerPath())
	if err == nil {
		name := strings.TrimSpace(string(marker))
		if name != "" && filepath.Base(name) == name && IsConfigFileName(name) {
			path := filepath.Join(p.ConfigDir, name)
			if info, statErr := os.Stat(path); statErr == nil && !info.IsDir() {
				return path, true, nil
			} else if statErr != nil && !os.IsNotExist(statErr) {
				return "", false, statErr
			}
		}
	} else if !os.IsNotExist(err) {
		return "", false, err
	}

	configs := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !IsConfigFileName(entry.Name()) {
			continue
		}
		configs = append(configs, filepath.Join(p.ConfigDir, entry.Name()))
	}
	if len(configs) == 0 {
		return "", false, nil
	}

	for _, path := range configs {
		if path == p.ConfigPath {
			return path, true, nil
		}
	}
	sort.Strings(configs)
	return configs[0], true, nil
}

func (p *Paths) ActiveConfigMarkerPath() string {
	return filepath.Join(p.ConfigDir, activeConfigMarkerName)
}

func (p *Paths) AppUpdateRestoreMarkerPath() string {
	return filepath.Join(p.DataDir, ".restore-core-after-update")
}

func (p *Paths) AppUpdateLockPath() string {
	return filepath.Join(p.DataDir, ".app-update.lock")
}

func (p *Paths) AppUpdateResultPath() string {
	return filepath.Join(p.DataDir, ".app-update-result")
}

func (p *Paths) AppUpdateLogPath() string {
	return filepath.Join(p.DataDir, ".app-update.log")
}

func (p *Paths) CacheFilePath() string {
	return filepath.Join(p.DataDir, "cache.db")
}

func (p *Paths) DNSMasqTakeoverStatePath() string {
	return filepath.Join(p.DataDir, ".dnsmasq-takeover.json")
}
