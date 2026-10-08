package rules

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	// Проверенные стабильные источники скомпилированных binary .srs правил
	GeoIPURL   = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-ru.srs"
	GeoSiteURL = "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-category-ru.srs"
	MinFileSize = 10 * 1024 // Минимальный размер файла (10 KB) для защиты от битых ответов / 404
)

// UpdateGeoBases скачивает свежие базы во временные файлы и атомарно заменяет их.
func UpdateGeoBases(ctx context.Context, geoIPTarget, geoSiteTarget string) error {
	client := &http.Client{
		Timeout: 45 * time.Second,
	}

	if err := downloadAtomic(ctx, client, GeoIPURL, geoIPTarget); err != nil {
		return fmt.Errorf("updating geoip-ru: %w", err)
	}

	if err := downloadAtomic(ctx, client, GeoSiteURL, geoSiteTarget); err != nil {
		return fmt.Errorf("updating geosite-category-ru: %w", err)
	}

	return nil
}

func downloadAtomic(ctx context.Context, client *http.Client, url, targetPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s failed: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad HTTP status %d from %s", resp.StatusCode, url)
	}

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, "srs-update-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName) // Удалит temp файл, если не переименован

	written, err := io.Copy(tmpFile, resp.Body)
	_ = tmpFile.Close()
	if err != nil {
		return fmt.Errorf("copying body: %w", err)
	}

	if written < MinFileSize {
		return fmt.Errorf("downloaded file too small (%d bytes), possibly corrupted or rate limited", written)
	}

	// Атомарная замена файла
	if err := os.Rename(tmpName, targetPath); err != nil {
		return fmt.Errorf("atomic rename %s -> %s: %w", tmpName, targetPath, err)
	}

	return nil
}
