package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"hysteria2-tray-client/internal/config"
	"hysteria2-tray-client/internal/profile"
)

func TestBuildConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	paths := &config.AppPaths{
		BaseDir:       tmpDir,
		ProfilesDir:   filepath.Join(tmpDir, "profiles"),
		DirectDomains: filepath.Join(tmpDir, "direct_domains.txt"),
		ProxyDomains:  filepath.Join(tmpDir, "proxy_domains.txt"),
		SingboxConfig: filepath.Join(tmpDir, "config.json"),
		GeoIPPath:     filepath.Join(tmpDir, "geoip-ru.srs"),
		GeoSitePath:   filepath.Join(tmpDir, "geosite-category-ru.srs"),
	}

	// Записываем тестовые правила
	_ = os.WriteFile(paths.DirectDomains, []byte("yandex.ru\nsber.ru\n"), 0o644)
	_ = os.WriteFile(paths.ProxyDomains, []byte("gemini.google.com\n"), 0o644)

	active := &profile.Item{
		ID:   "p1",
		Name: "Hysteria-Test",
		Outbound: profile.Outbound{
			Type:       "hysteria2",
			Tag:        "Hysteria-Test",
			Server:     "1.2.3.4",
			ServerPort: 443,
		},
	}

	all := []profile.Item{*active}

	cfgData, err := config.BuildConfig(paths, active, all)
	if err != nil {
		t.Fatalf("BuildConfig failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(cfgData, &parsed); err != nil {
		t.Fatalf("Unmarshal generated config failed: %v", err)
	}

	// Проверяем наличие experimental.clash_api
	exp, ok := parsed["experimental"].(map[string]any)
	if !ok {
		t.Fatalf("experimental section missing")
	}
	clash, ok := exp["clash_api"].(map[string]any)
	if !ok || clash["external_controller"] != "127.0.0.1:9090" {
		t.Errorf("clash_api external_controller mismatch: %v", clash)
	}

	// Проверяем наличие TUN inbound
	inbounds, ok := parsed["inbounds"].([]any)
	if !ok || len(inbounds) == 0 {
		t.Fatalf("inbounds missing")
	}
	tun := inbounds[0].(map[string]any)
	if tun["type"] != "tun" || tun["auto_route"] != true {
		t.Errorf("tun config invalid: %v", tun)
	}
}
