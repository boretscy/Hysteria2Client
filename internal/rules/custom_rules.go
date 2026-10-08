package rules

import (
	"bufio"
	"os"
	"strings"
)

// LoadDomainList читает домены из текстового файла, игнорируя комментарии (#) и пустые строки.
func LoadDomainList(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var domains []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		domains = append(domains, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return domains, nil
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
