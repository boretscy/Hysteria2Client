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
`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	domains, err := rules.LoadDomainList(filePath)
	if err != nil {
		t.Fatalf("LoadDomainList error: %v", err)
	}

	expected := []string{"yandex.ru", "sber.ru", "gosuslugi.ru"}
	if len(domains) != len(expected) {
		t.Fatalf("expected %d domains, got %d", len(expected), len(domains))
	}

	for i, d := range domains {
		if d != expected[i] {
			t.Errorf("domain[%d] = %s, want %s", i, d, expected[i])
		}
	}
}
