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

	// Записываем тестовые правила (включая IP/CIDR)
	_ = os.WriteFile(paths.DirectDomains, []byte("yandex.ru\nsber.ru\n10.7.7.182\n192.168.1.0/24\n"), 0o644)
	_ = os.WriteFile(paths.ProxyDomains, []byte("gemini.google.com\n8.8.8.8\n"), 0o644)

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

	// Проверяем устойчивость DNS конфигурации
	dns, ok := parsed["dns"].(map[string]any)
	if !ok {
		t.Fatalf("dns section missing")
	}
	if dns["timeout"] != "3s" {
		t.Errorf("expected dns.timeout=3s, got %v", dns["timeout"])
	}
	servers, ok := dns["servers"].([]any)
	if !ok || len(servers) == 0 {
		t.Fatalf("dns.servers missing")
	}
	tunnelDNS := servers[0].(map[string]any)
	if tunnelDNS["detour"] != "proxy" {
		t.Errorf("expected dns-tunnel detour=proxy, got %v", tunnelDNS["detour"])
	}

	// Проверяем наличие urltest ("auto") в outbounds
	outbounds, ok := parsed["outbounds"].([]any)
	if !ok || len(outbounds) < 2 {
		t.Fatalf("outbounds missing or too short: %v", outbounds)
	}

	proxySel := outbounds[0].(map[string]any)
	if proxySel["type"] != "selector" || proxySel["tag"] != "proxy" {
		t.Errorf("invalid proxy selector: %v", proxySel)
	}
	selList := proxySel["outbounds"].([]any)
	if len(selList) == 0 || selList[0] != "auto" {
		t.Errorf("expected selector first item to be 'auto', got %v", selList)
	}

	autoOutbound := outbounds[1].(map[string]any)
	if autoOutbound["type"] != "urltest" || autoOutbound["tag"] != "auto" {
		t.Errorf("invalid auto urltest outbound: %v", autoOutbound)
	}

	// Проверяем корректность IP-маршрутов в route
	route, ok := parsed["route"].(map[string]any)
	if !ok {
		t.Fatalf("route section missing")
	}
	rulesList, ok := route["rules"].([]any)
	if !ok {
		t.Fatalf("route.rules missing")
	}

	foundDirectIP := false
	for _, r := range rulesList {
		ruleMap := r.(map[string]any)
		if cidr, ok := ruleMap["ip_cidr"].([]any); ok && ruleMap["outbound"] == "direct" {
			for _, c := range cidr {
				if c == "10.7.7.182/32" {
					foundDirectIP = true
				}
			}
		}
	}
	if !foundDirectIP {
		t.Errorf("expected 10.7.7.182/32 in direct route rules")
	}
}
