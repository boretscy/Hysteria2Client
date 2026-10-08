package core

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"hysteria2-tray-client/internal/config"
	"hysteria2-tray-client/internal/profile"
)

// State хранит текущее состояние клиента.
type State struct {
	TunnelEnabled bool
	ActiveNode    string
	IsAlive       bool
}

// Controller управляет жизненным циклом и командами туннеля.
type Controller struct {
	mu          sync.RWMutex
	logger      *slog.Logger
	paths       *config.AppPaths
	profManager *profile.Manager
	clashClient *ClashClient
	state       State
}

// NewController создает контроллер.
func NewController(logger *slog.Logger, paths *config.AppPaths, profManager *profile.Manager) *Controller {
	return &Controller{
		logger:      logger,
		paths:       paths,
		profManager: profManager,
		clashClient: NewClashClient("127.0.0.1:9090"),
		state: State{
			TunnelEnabled: true,
			ActiveNode:    "hysteria",
		},
	}
}

// GetState возвращает снимок текущего состояния.
func (c *Controller) GetState() State {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// CheckStatus опрашивает sing-box и обновляет статус.
func (c *Controller) CheckStatus(ctx context.Context) State {
	c.mu.Lock()
	defer c.mu.Unlock()

	alive := c.clashClient.IsAlive(ctx)
	c.state.IsAlive = alive

	if alive {
		now, err := c.clashClient.GetActiveOutbound(ctx, "proxy")
		if err == nil {
			c.state.ActiveNode = now
			c.state.TunnelEnabled = (now != "direct")
		}
	}

	return c.state
}

// ToggleTunnel включает или выключает туннель (через режим Direct).
func (c *Controller) ToggleTunnel(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.state.IsAlive {
		return fmt.Errorf("sing-box daemon is not reachable on 127.0.0.1:9090")
	}

	if c.state.TunnelEnabled {
		// Ставим на паузу -> переключаем селектор в direct
		if err := c.clashClient.SelectOutbound(ctx, "proxy", "direct"); err != nil {
			return err
		}
		c.state.TunnelEnabled = false
		c.state.ActiveNode = "direct"
		c.logger.Info("tunnel paused (switched to direct)")
	} else {
		// Включаем туннель -> переключаем селектор на сохраненный профиль или hysteria
		active := c.profManager.Active()
		target := "hysteria"
		if active != nil && active.Name != "" {
			target = active.Name
		}
		if err := c.clashClient.SelectOutbound(ctx, "proxy", target); err != nil {
			// Fallback на hysteria если выбранная нода сбоит
			_ = c.clashClient.SelectOutbound(ctx, "proxy", "hysteria")
			target = "hysteria"
		}
		c.state.TunnelEnabled = true
		c.state.ActiveNode = target
		c.logger.Info("tunnel resumed", slog.String("node", target))
	}

	return nil
}

// SwitchProfile безопасно переключает узел на указанный профиль с Pre-flight валидацией.
func (c *Controller) SwitchProfile(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.profManager.SetActive(id); err != nil {
		return err
	}

	active := c.profManager.Active()
	if active == nil {
		return fmt.Errorf("no active profile")
	}

	if !c.state.IsAlive {
		return fmt.Errorf("sing-box daemon is not reachable")
	}

	// 1. Пытаемся переключить селектор
	err := c.clashClient.SelectOutbound(ctx, "proxy", active.Name)
	if err != nil {
		// Если ноды еще нет в селекторе (только что импортирована),
		// обновляем конфиг и перезапускаем демон
		c.logger.Info("node not in running selector, reloading config and restarting daemon", slog.String("node", active.Name))
		if syncErr := c.SyncConfigFile(); syncErr != nil {
			return syncErr
		}
		// Перезапуск системного демона через launchctl kickstart
		_ = exec.Command("launchctl", "kickstart", "-k", "system/com.singbox.tunnel").Run()
		time.Sleep(1 * time.Second)

		// Повторная попытка переключения
		retryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err2 := c.clashClient.SelectOutbound(retryCtx, "proxy", active.Name); err2 != nil {
			return fmt.Errorf("failed to select node %s: %w", active.Name, err2)
		}
	}

	// 2. Pre-flight тест доступности активной ноды (проверяем реальный пинг)
	testCtx, testCancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer testCancel()

	delay, testErr := c.clashClient.TestDelay(testCtx, active.Name, "https://www.gstatic.com/generate_204", 3000)
	if testErr != nil || delay == 0 {
		c.logger.Warn("selected node failed health check, rolling back to safe hysteria", slog.String("node", active.Name), slog.Any("error", testErr))
		// ОТКАТ на гарантированную hysteria!
		_ = c.clashClient.SelectOutbound(context.Background(), "proxy", "hysteria")
		c.state.ActiveNode = "hysteria"
		c.state.TunnelEnabled = true
		return fmt.Errorf("узел '%s' недоступен (ошибка соединения / Reality). Трафик откачен на Hysteria2", active.Name)
	}

	c.state.ActiveNode = active.Name
	c.state.TunnelEnabled = true
	c.logger.Info("successfully switched to node", slog.String("node", active.Name), slog.Int("delay_ms", delay))

	// Асинхронно сохраняем конфиг на диск
	return c.SyncConfigFile()
}

// SyncConfigFile генерирует и перезаписывает singbox config.json со всеми профилями.
func (c *Controller) SyncConfigFile() error {
	active := c.profManager.Active()
	all := c.profManager.List()

	data, err := config.BuildConfig(c.paths, active, all)
	if err != nil {
		return fmt.Errorf("building config: %w", err)
	}

	return config.WriteConfigAtomic(c.paths.SingboxConfig, data)
}

// ReloadDaemon перезагружает демон sing-box.
func (c *Controller) ReloadDaemon() error {
	if err := c.SyncConfigFile(); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "kickstart", "-k", "system/com.singbox.tunnel").Run()
	return nil
}

// PingActiveNode возвращает задержку до активного узла в миллисекундах.
func (c *Controller) PingActiveNode(ctx context.Context) (int, error) {
	c.mu.RLock()
	node := c.state.ActiveNode
	alive := c.state.IsAlive
	c.mu.RUnlock()

	if !alive || node == "" || node == "direct" {
		return 0, fmt.Errorf("no proxy node active")
	}

	return c.clashClient.TestDelay(ctx, node, "", 4000)
}
