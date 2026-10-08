package profile_test

import (
	"testing"

	"hysteria2-tray-client/internal/profile"
)

func TestParseURI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		uri         string
		wantErr     bool
		wantType    string
		wantTag     string
		wantServer  string
		wantPort    uint16
		checkTLS    func(t *testing.T, out profile.Outbound)
	}{
		{
			name:       "Hysteria2 standard link",
			uri:        "hysteria2://macbook:Hys2_secret@198.51.100.1:443/?sni=www.bing.com&insecure=1#Hysteria2-Primary",
			wantErr:    false,
			wantType:   "hysteria2",
			wantTag:    "Hysteria2-Primary",
			wantServer: "198.51.100.1",
			wantPort:   443,
			checkTLS: func(t *testing.T, out profile.Outbound) {
				if out.TLS == nil || !out.TLS.Enabled {
					t.Fatalf("expected TLS enabled")
				}
				if out.TLS.ServerName != "www.bing.com" {
					t.Errorf("got SNI %s, want www.bing.com", out.TLS.ServerName)
				}
				if !out.TLS.Insecure {
					t.Errorf("expected Insecure true")
				}
				if out.Password != "macbook:Hys2_secret" {
					t.Errorf("got password %s, want macbook:Hys2_secret", out.Password)
				}
			},
		},
		{
			name:       "VLESS Reality link",
			uri:        "vless://b831381d-6324-4d53-ad4f-8cda48b30811@vless.example.com:40001?security=reality&sni=fi.example.com&fp=chrome&pbk=x910283&sid=12ab&type=tcp&flow=xtls-rprx-vision#Finland-Reality",
			wantErr:    false,
			wantType:   "vless",
			wantTag:    "Finland-Reality",
			wantServer: "vless.example.com",
			wantPort:   40001,
			checkTLS: func(t *testing.T, out profile.Outbound) {
				if out.TLS == nil || out.TLS.Reality == nil {
					t.Fatalf("expected Reality config")
				}
				if out.TLS.Reality.PublicKey != "x910283" {
					t.Errorf("got pbk %s, want x910283", out.TLS.Reality.PublicKey)
				}
				if out.TLS.Reality.ShortID != "12ab" {
					t.Errorf("got sid %s, want 12ab", out.TLS.Reality.ShortID)
				}
				if out.TLS.UTLS.Fingerprint != "chrome" {
					t.Errorf("got fp %s, want chrome", out.TLS.UTLS.Fingerprint)
				}
				if out.Flow != "xtls-rprx-vision" {
					t.Errorf("got flow %s, want xtls-rprx-vision", out.Flow)
				}
			},
		},
		{
			name:    "Invalid scheme",
			uri:     "shadowsocks://abc@1.2.3.4:8388",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			item, err := profile.ParseURI(tt.uri)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseURI() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if item.Outbound.Type != tt.wantType {
				t.Errorf("Type = %s, want %s", item.Outbound.Type, tt.wantType)
			}
			if item.Name != tt.wantTag {
				t.Errorf("Name = %s, want %s", item.Name, tt.wantTag)
			}
			if item.Outbound.Server != tt.wantServer {
				t.Errorf("Server = %s, want %s", item.Outbound.Server, tt.wantServer)
			}
			if item.Outbound.ServerPort != tt.wantPort {
				t.Errorf("Port = %d, want %d", item.Outbound.ServerPort, tt.wantPort)
			}
			if tt.checkTLS != nil {
				tt.checkTLS(t, item.Outbound)
			}
		})
	}
}
