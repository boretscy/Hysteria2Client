package rules

import (
	"bufio"
	"net"
	"os"
	"strings"
)

// LoadDomainList читает домены и IP-адреса из текстового файла, игнорируя комментарии (#) и пустые строки.
// Разделяет записи на доменные имена (domains) и IP/CIDR подсети (ips).
func LoadDomainList(filePath string) (domains []string, ips []string, err error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, []string{}, nil
		}
		return nil, nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Проверяем, является ли запись IP или CIDR
		cleanIP := line
		if strings.Contains(line, "/") {
			if _, _, errCIDR := net.ParseCIDR(line); errCIDR == nil {
				ips = append(ips, line)
				continue
			}
		} else if parsedIP := net.ParseIP(cleanIP); parsedIP != nil {
			if parsedIP.To4() != nil {
				ips = append(ips, cleanIP+"/32")
			} else {
				ips = append(ips, cleanIP+"/128")
			}
			continue
		}

		domains = append(domains, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	return domains, ips, nil
}

// EnsureDefaultRuleFiles создает заготовки файлов исключений, если они еще не существуют.
func EnsureDefaultRuleFiles(directPath, proxyPath string) error {
	if _, err := os.Stat(directPath); os.IsNotExist(err) {
		defaultDirect := `# Direct Domains (всегда напрямую через локального провайдера)
# Добавляйте по одному домену на строку, например:
# yandex.ru
# sber.ru
# gosuslugi.ru
`
		_ = os.WriteFile(directPath, []byte(defaultDirect), 0o644)
	}

	if _, err := os.Stat(proxyPath); os.IsNotExist(err) {
		defaultProxy := `# Proxy Domains (всегда через активный зарубежный туннель)
# Добавляйте по одному домену на строку, например:
# gemini.google.com
# antigravity.google
# news.ycombinator.com
`
		_ = os.WriteFile(proxyPath, []byte(defaultProxy), 0o644)
	}
	return nil
}
