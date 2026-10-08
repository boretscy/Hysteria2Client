package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ClashClient взаимодействует с локальным REST API sing-box (Clash API).
type ClashClient struct {
	baseURL    string
	httpClient *http.Client
}

type ProxyDelayResponse struct {
	Delay int `json:"delay"`
}

type ProxiesResponse struct {
	Proxies map[string]ProxyDetail `json:"proxies"`
}

type ProxyDetail struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Now     string   `json:"now"`
	All     []string `json:"all"`
	History []struct {
		Time  string `json:"time"`
		Delay int    `json:"delay"`
	} `json:"history"`
}

// NewClashClient создает клиент к Clash API на указанном адресе (например, 127.0.0.1:9090).
func NewClashClient(addr string) *ClashClient {
	return &ClashClient{
		baseURL: fmt.Sprintf("http://%s", addr),
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// IsAlive проверяет, запущен ли и отвечает ли sing-box по REST API.
func (c *ClashClient) IsAlive(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/version", nil)
	if err != nil {
		return false
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetActiveOutbound возвращает имя текущего выбранного outbound в селекторе proxy.
func (c *ClashClient) GetActiveOutbound(ctx context.Context, selectorTag string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/proxies/%s", c.baseURL, selectorTag), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("clash api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var detail ProxyDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return "", err
	}
	return detail.Now, nil
}

// SelectOutbound переключает активный outbound в селекторе (например, "proxy" -> "Hysteria2" или "direct").
func (c *ClashClient) SelectOutbound(ctx context.Context, selectorTag, nodeName string) error {
	payload, err := json.Marshal(map[string]string{
		"name": nodeName,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/proxies/%s", c.baseURL, selectorTag), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("selecting outbound: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to select outbound, status: %d", resp.StatusCode)
	}
	return nil
}

// TestDelay замеряет задержку до указанного узла.
func (c *ClashClient) TestDelay(ctx context.Context, nodeName string, testURL string, timeoutMs int) (int, error) {
	if testURL == "" {
		testURL = "https://www.gstatic.com/generate_204"
	}
	url := fmt.Sprintf("%s/proxies/%s/delay?url=%s&timeout=%d", c.baseURL, nodeName, testURL, timeoutMs)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("delay test error, status: %d", resp.StatusCode)
	}

	var delayResp ProxyDelayResponse
	if err := json.NewDecoder(resp.Body).Decode(&delayResp); err != nil {
		return 0, err
	}
	return delayResp.Delay, nil
}
