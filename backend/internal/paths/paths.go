package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Paths struct {
	DataDir      string
	BinaryDir    string
	BinaryPath   string
	ConfigDir    string
	ConfigPath   string
	RulesDir     string
	GeoDir       string
	DownloadsDir string
	DBPath       string
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

	return &Paths{
		DataDir:      dataDir,
		BinaryDir:    binaryDir,
		BinaryPath:   filepath.Join(binaryDir, binaryName),
		ConfigDir:    configDir,
		ConfigPath:   filepath.Join(configDir, "config.json"),
		RulesDir:     rulesDir,
		GeoDir:       geoDir,
		DownloadsDir: downloadsDir,
		DBPath:       filepath.Join(dataDir, "ackwrap.db"),
	}
}

func (p *Paths) EnsureDirs() error {
	dirs := []string{p.DataDir, p.BinaryDir, p.ConfigDir, p.RulesDir, p.GeoDir, p.DownloadsDir}
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

	configs := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
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
