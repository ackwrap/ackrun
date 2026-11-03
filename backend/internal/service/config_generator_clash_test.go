package service

import (
	"encoding/json"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestGeneratedClashAPIStaysLocalWithoutDashboard(t *testing.T) {
	core := &configGeneratorCoreStub{}
	svc, db, _, _ := newScheduledRestartConfigGenerator(t, core)
	for _, secret := range []string{"", "synthetic-secret"} {
		var settings model.ExperimentalSettings
		if err := json.Unmarshal([]byte(`{
			"clash_api_port":"9091"
		}`), &settings); err != nil {
			t.Fatal(err)
		}
		settings.ClashAPISecret = secret
		if err := db.SetExperimentalSettings(&settings); err != nil {
			t.Fatal(err)
		}
		result, err := svc.GenerateCurrent()
		if err != nil {
			t.Fatal(err)
		}
		if !result.Valid {
			t.Fatalf("generated config is invalid: %s", result.Error)
		}
		clashAPI := result.Config["experimental"].(map[string]interface{})["clash_api"].(map[string]interface{})
		if clashAPI["external_controller"] != "127.0.0.1:9091" {
			t.Fatalf("controller = %v, want local listener", clashAPI["external_controller"])
		}
		for _, field := range []string{"external_ui", "external_ui_download_url"} {
			if _, exists := clashAPI[field]; exists {
				t.Errorf("disabled panel field is still generated: %s", field)
			}
		}
		if secret != "" && clashAPI["secret"] != "[REDACTED]" {
			t.Fatal("configured Clash API authentication was not included in the redacted preview")
		}
		stored, err := db.GetExperimentalSettings()
		if err != nil {
			t.Fatal(err)
		}
		if stored.ClashAPISecret != secret {
			t.Fatal("configured Clash API authentication was not preserved")
		}
	}
	if reloads, restarts := core.calls(); reloads != 0 || restarts != 0 {
		t.Fatal("generating a config must not restart the core")
	}
}
