package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParseURI разбирает ссылку vless://, hysteria2://, hy2:// и возвращает профиль Item.
func ParseURI(rawURI string) (*Item, error) {
	rawURI = strings.TrimSpace(rawURI)
	u, err := url.Parse(rawURI)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}

	name := u.Fragment
	if name == "" {
		name = fmt.Sprintf("%s-%s", u.Scheme, u.Hostname())
	}
	// Декодируем имя если оно в url-encoded формате
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}

	hash := sha256.Sum256([]byte(rawURI))
	id := hex.EncodeToString(hash[:8])

	switch strings.ToLower(u.Scheme) {
	case "hysteria2", "hy2":
		return parseHysteria2(id, name, rawURI, u)
	case "vless":
		return parseVLESS(id, name, rawURI, u)
	default:
		return nil, fmt.Errorf("unsupported protocol scheme: %s", u.Scheme)
	}
}

func parseHysteria2(id, name, rawURI string, u *url.URL) (*Item, error) {
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid hysteria2 port: %w", err)
	}

	// В Hysteria2 в поле userinfo может передаваться как "password", так и "user:password"
	var password string
	if u.User != nil {
		if pass, hasPass := u.User.Password(); hasPass {
			password = fmt.Sprintf("%s:%s", u.User.Username(), pass)
		} else {
			password = u.User.Username()
		}
	}

	q := u.Query()

	sni := q.Get("sni")
	if sni == "" {
		sni = host
	}

	insecure := q.Get("insecure") == "1" || strings.ToLower(q.Get("insecure")) == "true"

	out := Outbound{
		Type:       "hysteria2",
		Tag:        name,
		Server:     host,
		ServerPort: uint16(port),
		Password:   password,
		TLS: &TLSConfig{
			Enabled:    true,
			ServerName: sni,
			Insecure:   insecure,
		},
	}

	if obfs := q.Get("obfs"); obfs != "" {
		out.Obfs = &ObfsConfig{
			Type:     obfs,
			Password: q.Get("obfs-password"),
		}
	}

	return &Item{
		ID:       id,
		Name:     name,
		RawURI:   rawURI,
		Outbound: out,
	}, nil
}

func parseVLESS(id, name, rawURI string, u *url.URL) (*Item, error) {
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "443"
	}
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, fmt.Errorf("invalid vless port: %w", err)
	}

	var uuid string
	if u.User != nil {
		uuid = u.User.Username()
	}
	q := u.Query()

	security := strings.ToLower(q.Get("security"))
	sni := q.Get("sni")
	if sni == "" {
		sni = host
	}

	out := Outbound{
		Type:       "vless",
		Tag:        name,
		Server:     host,
		ServerPort: uint16(port),
		UUID:       uuid,
		Flow:       q.Get("flow"),
		Network:    q.Get("type"),
	}

	if security == "reality" || security == "tls" {
		fp := q.Get("fp")
		if fp == "" {
			fp = "chrome"
		}

		tlsCfg := &TLSConfig{
			Enabled:    true,
			ServerName: sni,
			Insecure:   q.Get("allowInsecure") == "1" || strings.ToLower(q.Get("allowInsecure")) == "true",
			UTLS: &UTLSConfig{
				Enabled:     true,
				Fingerprint: fp,
			},
		}

		if security == "reality" {
			tlsCfg.Reality = &RealityConfig{
				Enabled:   true,
				PublicKey: q.Get("pbk"),
				ShortID:   q.Get("sid"),
			}
		}
		out.TLS = tlsCfg
	}

	return &Item{
		ID:       id,
		Name:     name,
		RawURI:   rawURI,
		Outbound: out,
	}, nil
}
