package store

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"time"
)

// DisableDeviceProtectionForRepair updates only the existing protection flag.
// Recovery must not initialize a missing database or run unrelated migrations.
func DisableDeviceProtectionForRepair(dbPath string) error {
	if strings.TrimSpace(dbPath) == "" {
		return errors.New("防泄漏设置数据库路径为空")
	}
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`UPDATE app_settings SET value = 'false', updated_at = ? WHERE key = 'general.device_protection_enabled'`, time.Now().Unix())
	return err
}
