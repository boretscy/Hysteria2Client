package config

import (
	"os"
	"path/filepath"
	"runtime"
)

// AppPaths инкапсулирует системные и локальные пути приложения.
type AppPaths struct {
	BaseDir         string
	ProfilesDir     string
	DirectDomains   string
	ProxyDomains    string
	SingboxConfig   string
	GeoIPPath       string
	GeoSitePath     string
}

// ResolvePaths определяет пути в зависимости от ОС (macOS / Windows).
func ResolvePaths() (*AppPaths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Рабочая директория утилиты в App Support (macOS) или AppData/Roaming (Windows)
	var baseDir string
	var singboxConfig string
	var geoIPPath string
	var geoSitePath string

	switch runtime.GOOS {
	case "darwin":
		singboxDir := filepath.Join(homeDir, "Library", "Application Support", "singbox-tun")
		baseDir = filepath.Join(homeDir, "Library", "Application Support", "Hysteria2Client")
		singboxConfig = filepath.Join(singboxDir, "config.json")
		geoIPPath = filepath.Join(singboxDir, "rule-set", "geoip-ru.srs")
		geoSitePath = filepath.Join(singboxDir, "rule-set", "geosite-category-ru.srs")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		singboxDir := filepath.Join(appData, "singbox-tun")
		baseDir = filepath.Join(appData, "Hysteria2Client")
		singboxConfig = filepath.Join(singboxDir, "config.json")
		geoIPPath = filepath.Join(singboxDir, "rule-set", "geoip-ru.srs")
		geoSitePath = filepath.Join(singboxDir, "rule-set", "geosite-category-ru.srs")
	default:
		singboxDir := filepath.Join(homeDir, ".config", "singbox-tun")
		baseDir = filepath.Join(homeDir, ".config", "Hysteria2Client")
		singboxConfig = filepath.Join(singboxDir, "config.json")
		geoIPPath = filepath.Join(singboxDir, "rule-set", "geoip-ru.srs")
		geoSitePath = filepath.Join(singboxDir, "rule-set", "geosite-category-ru.srs")
	}

	_ = os.MkdirAll(baseDir, 0o755)
	profilesDir := filepath.Join(baseDir, "profiles")
	_ = os.MkdirAll(profilesDir, 0o755)

	return &AppPaths{
		BaseDir:       baseDir,
		ProfilesDir:   profilesDir,
		DirectDomains: filepath.Join(baseDir, "direct_domains.txt"),
		ProxyDomains:  filepath.Join(baseDir, "proxy_domains.txt"),
		SingboxConfig: singboxConfig,
		GeoIPPath:     geoIPPath,
		GeoSitePath:   geoSitePath,
	}, nil
}
