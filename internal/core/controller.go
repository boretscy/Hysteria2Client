package core

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

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
		// Включаем туннель -> переключаем селектор на сохраненный профиль
		active := c.profManager.Active()
		target := "direct"
		if active != nil {
			target = active.Name
		}
		if err := c.clashClient.SelectOutbound(ctx, "proxy", target); err != nil {
			return err
		}
		c.state.TunnelEnabled = true
		c.state.ActiveNode = target
		c.logger.Info("tunnel resumed", slog.String("node", target))
	}

	return nil
}

// SwitchProfile переключает узел на указанный профиль.
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

	if c.state.IsAlive {
		if err := c.clashClient.SelectOutbound(ctx, "proxy", active.Name); err != nil {
			c.logger.Warn("could not switch live via clash api, updating config on disk", slog.Any("error", err))
		} else {
			c.state.ActiveNode = active.Name
			c.state.TunnelEnabled = true
		}
	}

	// Сохраняем актуальный config.json для последующих запусков sing-box
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
