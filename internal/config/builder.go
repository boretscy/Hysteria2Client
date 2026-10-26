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
const (
	DefaultPrimaryTag = "Hysteria2-Primary"
	AutoFailoverTag   = "⚡️ Авто (URL-тест)"
	AutoOutboundTag   = "auto"
)

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
	Timeout  string      `json:"timeout,omitempty"`
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
	IPCIDR  []string `json:"ip_cidr,omitempty"`
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

// StandbySpainGRPCOutbound возвращает резервный gRPC Reality узел (ручной выбор, не входит в urltest auto).
func StandbySpainGRPCOutbound() map[string]any {
	return map[string]any{
		"type":        "vless",
		"tag":         "Spain-Madrid-VLESS-gRPC",
		"server":      "185.121.15.89",
		"server_port": 8443,
		"uuid":        "8744f5d1-72c7-4498-8731-a5c159155532",
		"flow":        "",
		"tls": map[string]any{
			"enabled":     true,
			"server_name": "gateway.icloud.com",
			"utls": map[string]any{
				"enabled":     true,
				"fingerprint": "chrome",
			},
			"reality": map[string]any{
				"enabled":    true,
				"public_key": "PtcKU30lv0MmFnlB3CxXypTJ3aK8ICpAEXHaT6BIQCE",
				"short_id":   "6b1a5acbef6591db",
			},
		},
		"transport": map[string]any{
			"type":                  "grpc",
			"service_name":          "grpc-vpn-tunnel",
			"idle_timeout":          "15s",
			"ping_timeout":          "15s",
			"permit_without_stream": true,
		},
	}
}

// BuildConfig собирает надежный, защищенный JSON для sing-box.
func BuildConfig(paths *AppPaths, activeProfile *profile.Item, allProfiles []profile.Item) ([]byte, error) {
	directDomains, directIPs, err := rules.LoadDomainList(paths.DirectDomains)
	if err != nil {
		directDomains = []string{}
		directIPs = []string{}
	}

	proxyDomains, proxyIPs, err := rules.LoadDomainList(paths.ProxyDomains)
	if err != nil {
		proxyDomains = []string{}
		proxyIPs = []string{}
	}

	// 1. Inbounds (Стабильный системный TUN)
	tunStack := "system"
	tunStrictRoute := true
	tunInterfaceName := ""

	if runtime.GOOS == "windows" {
		// На Windows: стек mixed (TCP через gVisor + UDP напрямую через Wintun)
		// strict_route: false исключает блокировку локального broadcast/DHCP
		tunStack = "mixed"
		tunStrictRoute = false
		tunInterfaceName = "SingBoxTUN"
	}

	tunInbound := map[string]any{
		"type":         "tun",
		"tag":          "tun-in",
		"address":      []string{"172.19.0.1/30"},
		"mtu":          1400,
		"auto_route":   true,
		"strict_route": tunStrictRoute,
		"stack":        tunStack,
	}
	if tunInterfaceName != "" {
		tunInbound["interface_name"] = tunInterfaceName
	}
	inbounds := []map[string]any{tunInbound}

	// 2. Outbounds: Сборка auto (urltest) и selector-группы "proxy"
	// Добавляем доступные узлы прокси
	var serverTags []string
	serverTags = append(serverTags, DefaultPrimaryTag)
	outboundNodes := []any{PrimaryHysteriaOutbound()}

	for _, p := range allProfiles {
		tag := p.Name
		if tag == DefaultPrimaryTag || tag == "hysteria" || tag == "direct" || tag == "proxy" || tag == AutoOutboundTag || tag == "Spain-Madrid-VLESS-gRPC" {
			continue
		}
		serverTags = append(serverTags, tag)
		outboundNodes = append(outboundNodes, p.Outbound)
	}

	// Добавляем резервный gRPC outbound:
	// Присутствует в outboundNodes и selectorOutbounds для ручного выбора.
	// НЕ ДОБАВЛЯЕТСЯ в serverTags / urltestOutbound (strict standby).
	grpcStandby := StandbySpainGRPCOutbound()
	grpcTag := grpcStandby["tag"].(string)
	outboundNodes = append(outboundNodes, grpcStandby)

	// Создаем группу автоматического выбора urltest ("auto")
	urltestOutbound := map[string]any{
		"type":      "urltest",
		"tag":       AutoOutboundTag,
		"outbounds": serverTags,
		"url":       "http://www.gstatic.com/generate_204",
		"interval":  "30s",
		"tolerance": 100,
	}

	// Сборка селектора: сначала "auto", затем конкретные узлы (включая gRPC), затем "direct" (пауза)
	selectorOutbounds := []string{AutoOutboundTag}
	selectorOutbounds = append(selectorOutbounds, serverTags...)
	selectorOutbounds = append(selectorOutbounds, grpcTag)
	selectorOutbounds = append(selectorOutbounds, "direct")

	// Дефолтная нода селектора: если пользователь явно выбрал профиль — ставим его, иначе по умолчанию "auto"
	defaultSelected := AutoOutboundTag
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

	finalOutbounds := []any{proxySelector, urltestOutbound}
	finalOutbounds = append(finalOutbounds, outboundNodes...)
	finalOutbounds = append(finalOutbounds, directOutbound)

	// 3. DNS: КРИТИЧЕСКАЯ ЗАЩИТА СЕТИ!
	// dns-tunnel detour привязан к селектору "proxy".
	// Если туннель на паузе ("direct") или переключен на другой профиль,
	// DNS-трафик корректно следует за выбранным узлом.
	dnsRules := []DNSRule{
		{
			RuleSet: []string{"geosite-category-ru"},
			Server:  "dns-direct",
		},
	}
	if len(directDomains) > 0 {
		dnsRules = append([]DNSRule{{Domain: directDomains, Server: "dns-direct"}}, dnsRules...)
	}
	if len(directIPs) > 0 {
		dnsRules = append([]DNSRule{{IPCIDR: directIPs, Server: "dns-direct"}}, dnsRules...)
	}

	dnsCfg := DNSConfig{
		Servers: []DNSServer{
			{
				Type:   "https",
				Tag:    "dns-tunnel",
				Server: "8.8.8.8",
				Detour: "proxy",
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
		Timeout:  "3s",
	}

	// 4. Route Rules
	routeRules := []map[string]any{
		{"action": "sniff"},
		{"protocol": "dns", "action": "hijack-dns"},
		{"ip_is_private": true, "outbound": "direct"},
		// Защита от утечки открытого DoH на паузе:
		// DoH запросы к Google DNS блокируются при попытке уйти напрямую (вне proxy)
		{
			"ip_cidr":  []string{"8.8.8.8/32", "8.8.4.4/32"},
			"outbound": "direct",
			"action":   "reject",
		},
	}

	// На Windows: изоляция процесса Antigravity.exe
	if runtime.GOOS == "windows" {
		routeRules = append(routeRules, map[string]any{
			"process_name": []string{"Antigravity.exe", "Antigravity", "antigravity.exe"},
			"outbound":     "proxy",
		})
	}

	// Пользовательские Proxy домены и IP
	if len(proxyDomains) > 0 {
		routeRules = append(routeRules, map[string]any{
			"domain_suffix": proxyDomains,
			"outbound":      "proxy",
		})
	}
	if len(proxyIPs) > 0 {
		routeRules = append(routeRules, map[string]any{
			"ip_cidr":  proxyIPs,
			"outbound": "proxy",
		})
	}

	// Пользовательские Direct домены и IP (включая одиночные IP вроде 10.7.7.182/32)
	if len(directDomains) > 0 {
		routeRules = append(routeRules, map[string]any{
			"domain_suffix": directDomains,
			"outbound":      "direct",
		})
	}
	if len(directIPs) > 0 {
		routeRules = append(routeRules, map[string]any{
			"ip_cidr":  directIPs,
			"outbound": "direct",
		})
	}

	// Базовые правила разделения РФ
	routeRules = append(routeRules,
		map[string]any{"rule_set": []string{"geosite-category-ru"}, "outbound": "direct"},
		map[string]any{"rule_set": []string{"geoip-ru"}, "outbound": "direct"},
	)

	clashSecret, _ := paths.GetOrCreateClashSecret()

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
				Secret:             clashSecret,
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
