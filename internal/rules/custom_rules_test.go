package rules_test

import (
	"os"
	"path/filepath"
	"testing"

	"hysteria2-tray-client/internal/rules"
)

func TestLoadDomainList(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "domains.txt")

	content := `# Комментарий
yandex.ru
  sber.ru  

# Пустые строки и еще коммент
gosuslugi.ru
10.7.7.182
192.168.1.0/24
`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	domains, ips, err := rules.LoadDomainList(filePath)
	if err != nil {
		t.Fatalf("LoadDomainList error: %v", err)
	}

	expectedDomains := []string{"yandex.ru", "sber.ru", "gosuslugi.ru"}
	if len(domains) != len(expectedDomains) {
		t.Fatalf("expected %d domains, got %d", len(expectedDomains), len(domains))
	}

	for i, d := range domains {
		if d != expectedDomains[i] {
			t.Errorf("domain[%d] = %s, want %s", i, d, expectedDomains[i])
		}
	}

	expectedIPs := []string{"10.7.7.182/32", "192.168.1.0/24"}
	if len(ips) != len(expectedIPs) {
		t.Fatalf("expected %d ips, got %d: %v", len(expectedIPs), len(ips), ips)
	}
	for i, ip := range ips {
		if ip != expectedIPs[i] {
			t.Errorf("ip[%d] = %s, want %s", i, ip, expectedIPs[i])
		}
	}
}
