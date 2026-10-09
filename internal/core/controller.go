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
	mu             sync.RWMutex
	logger         *slog.Logger
	paths          *config.AppPaths
	profManager    *profile.Manager
	clashClient    *ClashClient
	state          State
	lastActiveNode string // Запоминает узел перед уходом в паузу
}

// NewController создает контроллер.
func NewController(logger *slog.Logger, paths *config.AppPaths, profManager *profile.Manager) *Controller {
	return &Controller{
		logger:         logger,
		paths:          paths,
		profManager:    profManager,
		clashClient:    NewClashClient("127.0.0.1:9090"),
		lastActiveNode: config.DefaultPrimaryTag,
		state: State{
			TunnelEnabled: true,
			ActiveNode:    config.DefaultPrimaryTag,
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
			if now != "direct" {
				c.lastActiveNode = now
			}
		}
	}

	return c.state
}

// ToggleTunnel включает или выключает туннель (через режим Direct).
// Не выбирает случайные ноды — строго возвращает последний активный узел.
func (c *Controller) ToggleTunnel(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.state.IsAlive {
		return fmt.Errorf("sing-box daemon is not reachable on 127.0.0.1:9090")
	}

	if c.state.TunnelEnabled {
		// Сохраняем перед паузой текущий узел
		if c.state.ActiveNode != "" && c.state.ActiveNode != "direct" {
			c.lastActiveNode = c.state.ActiveNode
		}
		// Переключаем селектор в direct
		if err := c.clashClient.SelectOutbound(ctx, "proxy", "direct"); err != nil {
			return err
		}
		c.state.TunnelEnabled = false
		c.state.ActiveNode = "direct"
		c.logger.Info("tunnel paused (switched to direct)")
	} else {
		// Возобновляем туннель строго на сохраненный lastActiveNode (или Hysteria2-Primary)
		target := c.lastActiveNode
		if target == "" || target == "direct" {
			target = config.DefaultPrimaryTag
		}

		if err := c.clashClient.SelectOutbound(ctx, "proxy", target); err != nil {
			// Если сохраненный узел по какой-то причине отсутствует в селекторе — только тогда fallback на Primary
			c.logger.Warn("target node unavailable, falling back to default primary", slog.String("target", target), slog.Any("error", err))
			if errFallback := c.clashClient.SelectOutbound(ctx, "proxy", config.DefaultPrimaryTag); errFallback != nil {
				return errFallback
			}
			target = config.DefaultPrimaryTag
		}
		c.state.TunnelEnabled = true
		c.state.ActiveNode = target
		c.lastActiveNode = target
		c.logger.Info("tunnel resumed", slog.String("node", target))
	}

	return nil
}

// SwitchProfile переключает узел строго по явному выбору пользователя.
// БЕЗ автоматических откатов и блокирующих проверок.
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

	// Переключаем селектор в ядре sing-box
	err := c.clashClient.SelectOutbound(ctx, "proxy", active.Name)
	if err != nil {
		// Если ноды еще нет в селекторе ядра (только что добавлена),
		// сохраняем конфигурацию и делаем перезапуск демона
		c.logger.Info("node not in running selector, reloading daemon", slog.String("node", active.Name))
		if syncErr := c.SyncConfigFile(); syncErr != nil {
			return syncErr
		}
		_ = exec.Command("launchctl", "kickstart", "-k", "system/com.singbox.tunnel").Run()
		time.Sleep(1 * time.Second)

		// Повторяем выбор
		retryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err2 := c.clashClient.SelectOutbound(retryCtx, "proxy", active.Name); err2 != nil {
			return fmt.Errorf("failed to select node %s: %w", active.Name, err2)
		}
	}

	// Фиксируем успешный выбор пользователя
	c.state.ActiveNode = active.Name
	c.state.TunnelEnabled = true
	c.lastActiveNode = active.Name
	c.logger.Info("user selected node switched successfully", slog.String("node", active.Name))

	// Асинхронно сохраняем конфиг на диск
	return c.SyncConfigFile()
}

// SafeExit гарантирует безопасное состояние туннеля при выходе из GUI клиента.
// Если туннель был на паузе ("direct"), возвращает рабочий туннель, чтобы машина не осталась без защиты.
func (c *Controller) SafeExit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.state.IsAlive {
		return
	}

	// Если пользователь выходил при выключенном туннеле — возвращаем Hysteria2-Primary
	if !c.state.TunnelEnabled || c.state.ActiveNode == "direct" {
		c.logger.Info("safe exit: ensuring tunnel is active on default primary before quitting")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = c.clashClient.SelectOutbound(ctx, "proxy", config.DefaultPrimaryTag)
	}
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

	return c.clashClient.TestDelay(ctx, node, "", 10000)
}
