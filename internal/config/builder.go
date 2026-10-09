package config

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"hysteria2-tray-client/internal/profile"
	"hysteria2-tray-client/internal/rules"
)

// Имя надежного канонического профиля контура
const DefaultPrimaryTag = "Hysteria2-Primary"

// SingboxFullConfig представляет структуру полного config.json для sing-box 1.10+.
type SingboxFullConfig struct {
	Log          LogConfig              `json:"log"`
	DNS          DNSConfig              `json:"dns"`
	Inbounds     []map[string]any       `json:"inbounds"`
	Outbounds    []any                  `json:"outbounds"`
	Route        RouteConfig            `json:"route"`
	Experimental *ExperimentalConfig    `json:"experimental,omitempty"`
}

type LogConfig struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type DNSConfig struct {
	Servers  []DNSServer `json:"servers"`
	Rules    []DNSRule   `json:"rules"`
	Final    string      `json:"final"`
	Strategy string      `json:"strategy"`
}

type DNSServer struct {
	Type   string `json:"type"`
	Tag    string `json:"tag"`
	Server string `json:"server"`
	Detour string `json:"detour,omitempty"`
}

type DNSRule struct {
	RuleSet []string `json:"rule_set,omitempty"`
	Domain  []string `json:"domain,omitempty"`
	Server  string   `json:"server"`
}

type RouteConfig struct {
	Rules                 []map[string]any `json:"rules"`
	RuleSet               []RuleSetConfig  `json:"rule_set"`
	Final                 string           `json:"final"`
	AutoDetectInterface   bool             `json:"auto_detect_interface"`
	DefaultDomainResolver map[string]any   `json:"default_domain_resolver"`
}

type RuleSetConfig struct {
	Type   string `json:"type"`
	Tag    string `json:"tag"`
	Format string `json:"format"`
	Path   string `json:"path"`
}

type ExperimentalConfig struct {
	ClashAPI *ClashAPIConfig `json:"clash_api,omitempty"`
}

type ClashAPIConfig struct {
	ExternalController string `json:"external_controller"`
	Secret             string `json:"secret,omitempty"`
}

// PrimaryHysteriaOutbound возвращает канонический outbound нашего боевого VPS.
func PrimaryHysteriaOutbound() profile.Outbound {
	return profile.Outbound{
		Type:       "hysteria2",
		Tag:        DefaultPrimaryTag,
		Server:     "13.143.251.197",
		ServerPort: 443,
		Password:   "macbook:Hys2_214614ae8ec9ba15d24b57f2",
		TLS: &profile.TLSConfig{
			Enabled:    true,
			ServerName: "www.bing.com",
			Insecure:   true,
		},
	}
}

// BuildConfig собирает надежный, защищенный JSON для sing-box.
func BuildConfig(paths *AppPaths, activeProfile *profile.Item, allProfiles []profile.Item) ([]byte, error) {
	directDomains, err := rules.LoadDomainList(paths.DirectDomains)
	if err != nil {
		directDomains = []string{}
	}

	proxyDomains, err := rules.LoadDomainList(paths.ProxyDomains)
	if err != nil {
		proxyDomains = []string{}
	}

	// 1. Inbounds (Стабильный системный TUN)
	inbounds := []map[string]any{
		{
			"type":         "tun",
			"tag":          "tun-in",
			"address":      []string{"172.19.0.1/30"},
			"mtu":          1400,
			"auto_route":   true,
			"strict_route": true,
			"stack":        "system",
		},
	}

	// 2. Outbounds: Сборка selector-группы "proxy"
	selectorOutbounds := []string{DefaultPrimaryTag}
	outboundNodes := []any{PrimaryHysteriaOutbound()}

	// Добавляем импортированные профили (исключая дубликаты основного тега и системных слов)
	for _, p := range allProfiles {
		tag := p.Name
		if tag == DefaultPrimaryTag || tag == "hysteria" || tag == "direct" || tag == "proxy" {
			continue
		}
		selectorOutbounds = append(selectorOutbounds, tag)
		outboundNodes = append(outboundNodes, p.Outbound)
	}

	// Опция "direct" в селекторе для мгновенной паузы туннеля
	selectorOutbounds = append(selectorOutbounds, "direct")

	// Дефолтная нода селектора: если выбран профиль — он, иначе всегда канонический Hysteria2-Primary
	defaultSelected := DefaultPrimaryTag
	if activeProfile != nil && activeProfile.Name != "" {
		for _, tag := range selectorOutbounds {
			if tag == activeProfile.Name {
				defaultSelected = tag
				break
			}
		}
	}

	proxySelector := map[string]any{
		"type":      "selector",
		"tag":       "proxy",
		"outbounds": selectorOutbounds,
		"default":   defaultSelected,
	}

	directOutbound := map[string]any{
		"type": "direct",
		"tag":  "direct",
	}

	finalOutbounds := []any{proxySelector}
	finalOutbounds = append(finalOutbounds, outboundNodes...)
	finalOutbounds = append(finalOutbounds, directOutbound)

	// 3. DNS: КРИТИЧЕСКАЯ ЗАЩИТА СЕТИ!
	// dns-tunnel detour ЖЕСТКО привязан к "Hysteria2-Primary".
	// Даже если пользователь включит сбойную ноду — системный DNS НИКОГДА не упадет!
	dnsRules := []DNSRule{
		{
			RuleSet: []string{"geosite-category-ru"},
			Server:  "dns-direct",
		},
	}
	if len(directDomains) > 0 {
		dnsRules = append([]DNSRule{{Domain: directDomains, Server: "dns-direct"}}, dnsRules...)
	}

	dnsCfg := DNSConfig{
		Servers: []DNSServer{
			{
				Type:   "https",
				Tag:    "dns-tunnel",
				Server: "8.8.8.8",
				Detour: DefaultPrimaryTag, // Канонический надежный тег!
			},
			{
				Type:   "udp",
				Tag:    "dns-direct",
				Server: "77.88.8.8",
			},
		},
		Rules:    dnsRules,
		Final:    "dns-tunnel",
		Strategy: "prefer_ipv4",
	}

	// 4. Route Rules
	routeRules := []map[string]any{
		{"action": "sniff"},
		{"protocol": "dns", "action": "hijack-dns"},
		{"ip_is_private": true, "outbound": "direct"},
	}

	// На Windows: изоляция процесса Antigravity.exe
	if runtime.GOOS == "windows" {
		routeRules = append(routeRules, map[string]any{
			"process_name": []string{"Antigravity.exe", "Antigravity", "antigravity.exe"},
			"outbound":     "proxy",
		})
	}

	// Пользовательские Proxy домены
	if len(proxyDomains) > 0 {
		routeRules = append(routeRules, map[string]any{
			"domain_suffix": proxyDomains,
			"outbound":      "proxy",
		})
	}

	// Пользовательские Direct домены
	if len(directDomains) > 0 {
		routeRules = append(routeRules, map[string]any{
			"domain_suffix": directDomains,
			"outbound":      "direct",
		})
	}

	// Базовые правила разделения РФ
	routeRules = append(routeRules,
		map[string]any{"rule_set": []string{"geosite-category-ru"}, "outbound": "direct"},
		map[string]any{"rule_set": []string{"geoip-ru"}, "outbound": "direct"},
	)

	fullCfg := SingboxFullConfig{
		Log: LogConfig{
			Level:     "info",
			Timestamp: true,
		},
		DNS:       dnsCfg,
		Inbounds:  inbounds,
		Outbounds: finalOutbounds,
		Route: RouteConfig{
			Rules: routeRules,
			RuleSet: []RuleSetConfig{
				{
					Type:   "local",
					Tag:    "geoip-ru",
					Format: "binary",
					Path:   paths.GeoIPPath,
				},
				{
					Type:   "local",
					Tag:    "geosite-category-ru",
					Format: "binary",
					Path:   paths.GeoSitePath,
				},
			},
			Final:               "proxy",
			AutoDetectInterface: true,
			DefaultDomainResolver: map[string]any{
				"server":   "dns-direct",
				"strategy": "prefer_ipv4",
			},
		},
		Experimental: &ExperimentalConfig{
			ClashAPI: &ClashAPIConfig{
				ExternalController: "127.0.0.1:9090",
			},
		},
	}

	return json.MarshalIndent(fullCfg, "", "  ")
}

// WriteConfigAtomic сохраняет конфиг во временный файл и производит атомарную замену.
func WriteConfigAtomic(targetPath string, data []byte) error {
	tmpPath := targetPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("writing tmp config: %w", err)
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("atomic rename %s -> %s: %w", tmpPath, targetPath, err)
	}
	return nil
}
