package core

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
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

	// Watchdog переменные
	deadSince      time.Time
	lastRestartAt  time.Time
}

// NewController создает контроллер.
func NewController(logger *slog.Logger, paths *config.AppPaths, profManager *profile.Manager) *Controller {
	secret, _ := paths.GetOrCreateClashSecret()
	return &Controller{
		logger:         logger,
		paths:          paths,
		profManager:    profManager,
		clashClient:    NewClashClient("127.0.0.1:9090", secret),
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
// Включает watchdog: если API демона недоступен более 15 секунд, автоматически
// инициирует перезапуск системной службы (с cooldown защитой).
func (c *Controller) CheckStatus(ctx context.Context) State {
	c.mu.Lock()
	defer c.mu.Unlock()

	alive := c.clashClient.IsAlive(ctx)
	c.state.IsAlive = alive

	now := time.Now()
	if alive {
		c.deadSince = time.Time{} // сбрасываем счетчик недоступности
		node, err := c.clashClient.GetActiveOutbound(ctx, "proxy")
		if err == nil {
			c.state.ActiveNode = node
			c.state.TunnelEnabled = (node != "direct")
			if node != "direct" {
				c.lastActiveNode = node
			}
		}
	} else {
		// Служба не отвечает
		if c.deadSince.IsZero() {
			c.deadSince = now
			c.logger.Warn("sing-box daemon is not responding to REST API, starting watchdog timer")
		} else {
			deadDuration := now.Sub(c.deadSince)
			// Если API недоступен > 15 секунд и с прошлого рестарта прошло > 30 секунд (cooldown)
			if deadDuration >= 15*time.Second && (c.lastRestartAt.IsZero() || now.Sub(c.lastRestartAt) >= 30*time.Second) {
				c.logger.Error("watchdog: sing-box daemon unreachable for >15s, triggering automatic service restart",
					slog.Duration("unreachable_for", deadDuration))
				c.lastRestartAt = now
				go c.restartDaemonInternal()
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

// SwitchToAuto переключает селектор proxy в режим auto (urltest failover).
func (c *Controller) SwitchToAuto(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.state.IsAlive {
		return fmt.Errorf("sing-box daemon is not reachable")
	}

	if err := c.clashClient.SelectOutbound(ctx, "proxy", "auto"); err != nil {
		c.logger.Info("auto outbound not in running selector, reloading daemon")
		if syncErr := c.SyncConfigFile(); syncErr != nil {
			return syncErr
		}
		c.restartDaemonInternal()
		time.Sleep(1 * time.Second)

		retryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err2 := c.clashClient.SelectOutbound(retryCtx, "proxy", "auto"); err2 != nil {
			return fmt.Errorf("failed to select auto: %w", err2)
		}
	}

	c.state.ActiveNode = "auto"
	c.state.TunnelEnabled = true
	c.lastActiveNode = "auto"
	c.logger.Info("switched to auto failover mode")

	return c.SyncConfigFile()
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
		c.restartDaemonInternal()
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

// restartDaemonInternal перезапускает системный демон/службу кроссплатформенно.
func (c *Controller) restartDaemonInternal() {
	if runtime.GOOS == "windows" {
		// На Windows: служба перезапускается нативно без UAC благодаря правам DACL (sc sdset)
		c.logger.Info("restarting sing-box Windows service via sc.exe")
		cmdStop := exec.Command("sc.exe", "stop", "sing-box")
		if out, err := cmdStop.CombinedOutput(); err != nil {
			c.logger.Warn("sc.exe stop output", slog.String("output", string(out)), slog.Any("error", err))
		}
		time.Sleep(1 * time.Second)
		cmdStart := exec.Command("sc.exe", "start", "sing-box")
		if out, err := cmdStart.CombinedOutput(); err != nil {
			c.logger.Warn("sc.exe start output", slog.String("output", string(out)), slog.Any("error", err))
		} else {
			c.logger.Info("sing-box Windows service restart command issued successfully")
		}
	} else {
		// На macOS: launchctl kickstart
		c.logger.Info("restarting sing-box macOS daemon via launchctl kickstart")
		cmd := exec.Command("launchctl", "kickstart", "-k", "system/com.singbox.tunnel")
		if out, err := cmd.CombinedOutput(); err != nil {
			c.logger.Warn("launchctl kickstart output", slog.String("output", string(out)), slog.Any("error", err))
		} else {
			c.logger.Info("launchctl kickstart executed successfully")
		}
	}
}

// ReloadDaemon перезагружает демон sing-box.
func (c *Controller) ReloadDaemon() error {
	if err := c.SyncConfigFile(); err != nil {
		return err
	}
	c.restartDaemonInternal()
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
