package profile

// TLSConfig описывает TLS/Reality настройки outbound в sing-box.
type TLSConfig struct {
	Enabled    bool           `json:"enabled,omitempty"`
	ServerName string         `json:"server_name,omitempty"`
	Insecure   bool           `json:"insecure,omitempty"`
	UTLS       *UTLSConfig    `json:"utls,omitempty"`
	Reality    *RealityConfig `json:"reality,omitempty"`
}

// UTLSConfig содержит параметры uTLS fingerprint.
type UTLSConfig struct {
	Enabled     bool   `json:"enabled,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// RealityConfig содержит параметры VLESS Reality.
type RealityConfig struct {
	Enabled   bool   `json:"enabled,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	ShortID   string `json:"short_id,omitempty"`
}

// Outbound описывает структуру исходящего прокси в формате sing-box 1.10+.
type Outbound struct {
	Type       string      `json:"type"`
	Tag        string      `json:"tag"`
	Server     string      `json:"server,omitempty"`
	ServerPort uint16      `json:"server_port,omitempty"`
	UUID       string      `json:"uuid,omitempty"`
	Password   string      `json:"password,omitempty"`
	Flow       string      `json:"flow,omitempty"`
	Network    string      `json:"network,omitempty"`
	TLS        *TLSConfig  `json:"tls,omitempty"`
	UpMbps     int         `json:"up_mbps,omitempty"`
	DownMbps   int         `json:"down_mbps,omitempty"`
	Obfs       *ObfsConfig `json:"obfs,omitempty"`
}

// ObfsConfig описывает обфускацию для Hysteria2.
type ObfsConfig struct {
	Type     string `json:"type,omitempty"`
	Password string `json:"password,omitempty"`
}

// Item представляет сохраненный профиль в менеджере профилей.
type Item struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	RawURI   string   `json:"raw_uri"`
	Outbound Outbound `json:"outbound"`
}
