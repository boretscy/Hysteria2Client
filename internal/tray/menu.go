package tray

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"

	"hysteria2-tray-client/internal/config"
	"hysteria2-tray-client/internal/core"
	"hysteria2-tray-client/internal/profile"
	"hysteria2-tray-client/internal/rules"
)

// App инкапсулирует состояние и интерфейс трея.
type App struct {
	logger      *slog.Logger
	paths       *config.AppPaths
	profManager *profile.Manager
	controller  *core.Controller

	mu             sync.Mutex
	statusItem     *systray.MenuItem
	toggleItem     *systray.MenuItem
	profileSubmenu *systray.MenuItem
	profileItems   map[string]*systray.MenuItem
	importItem     *systray.MenuItem
	updateGeoItem  *systray.MenuItem
	openRulesItem  *systray.MenuItem
	quitItem       *systray.MenuItem
}

// NewApp создает экземпляр трей-приложения.
func NewApp(logger *slog.Logger, paths *config.AppPaths, profManager *profile.Manager, controller *core.Controller) *App {
	return &App{
		logger:       logger,
		paths:        paths,
		profManager:  profManager,
		controller:   controller,
		profileItems: make(map[string]*systray.MenuItem),
	}
}

// Run запускает systray цикл событий.
func (a *App) Run() {
	systray.Run(a.onReady, a.onExit)
}

func (a *App) onReady() {
	systray.SetTitle("")
	systray.SetTooltip("sing-box Client (Hysteria2 & Reality)")
	systray.SetIcon(IconActive())

	a.statusItem = systray.AddMenuItem("Подключение: Проверка...", "Текущее состояние")
	a.statusItem.Disable()

	a.toggleItem = systray.AddMenuItem("Туннель: Включен", "Переключить туннель (Direct / Proxy)")
	systray.AddSeparator()

	a.profileSubmenu = systray.AddMenuItem("Профили подключения", "Выбор активного сервера")
	a.refreshProfileMenu()

	a.importItem = systray.AddMenuItem("Импорт профиля из буфера...", "Добавить ссылку vless:// или hysteria2://")
	systray.AddSeparator()

	a.updateGeoItem = systray.AddMenuItem("Обновить базы GeoIP / GeoSite", "Загрузить свежие .srs правила с GitHub")
	a.openRulesItem = systray.AddMenuItem("Открыть папку исключений", "Редактировать direct_domains и proxy_domains")
	systray.AddSeparator()

	a.quitItem = systray.AddMenuItem("Выход", "Закрыть клиент")

	// Фоновый опрос статуса демона каждые 3 секунды
	go a.statusLoop()

	// Обработчик событий кликов в трее
	go a.eventLoop()
}

func (a *App) onExit() {
	a.logger.Info("systray exited")
}

func (a *App) refreshProfileMenu() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Очищаем старые пункты
	for _, item := range a.profileItems {
		item.Hide()
	}
	a.profileItems = make(map[string]*systray.MenuItem)

	profiles := a.profManager.List()
	state := a.controller.GetState()
	for _, p := range profiles {
		prof := p
		title := fmt.Sprintf("[%s] %s", prof.Outbound.Type, prof.Name)
		item := a.profileSubmenu.AddSubMenuItem(title, prof.RawURI)
		if state.TunnelEnabled && (prof.Name == state.ActiveNode || (state.ActiveNode == "hysteria" && prof.Outbound.Type == "hysteria2")) {
			item.Check()
		} else {
			item.Uncheck()
		}
		a.profileItems[prof.ID] = item

		go func(id string, menuItem *systray.MenuItem) {
			for range menuItem.ClickedCh {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := a.controller.SwitchProfile(ctx, id); err != nil {
					_ = beeep.Alert("Ошибка переключения", err.Error(), "")
				} else {
					a.updateUIState()
				}
				cancel()
			}
		}(prof.ID, item)
	}
}

func (a *App) statusLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		a.controller.CheckStatus(ctx)
		cancel()
		a.updateUIState()
	}
}

func (a *App) updateUIState() {
	state := a.controller.GetState()

	if !state.IsAlive {
		systray.SetIcon(IconDisconnected())
		a.statusItem.SetTitle("Демон sing-box: Недоступен (127.0.0.1:9090)")
		a.toggleItem.SetTitle("Туннель: Ошибка соединения")
		return
	}

	if state.TunnelEnabled {
		systray.SetIcon(IconActive())
		a.statusItem.SetTitle(fmt.Sprintf("Активен: %s", state.ActiveNode))
		a.toggleItem.SetTitle("Туннель: Включен (Нажмите для паузы)")
	} else {
		systray.SetIcon(IconPaused())
		a.statusItem.SetTitle("Режим: Direct (Пауза)")
		a.toggleItem.SetTitle("Туннель: Выключен (Нажмите для включения)")
	}

	// Обновляем чекбоксы в списке профилей по РЕАЛЬНОМУ активному узлу в sing-box
	a.mu.Lock()
	for _, p := range a.profManager.List() {
		if item, ok := a.profileItems[p.ID]; ok {
			// Если имя ноды совпадает с активным узлом селектора sing-box и туннель включен
			if state.TunnelEnabled && (p.Name == state.ActiveNode || (state.ActiveNode == "hysteria" && p.Outbound.Type == "hysteria2")) {
				item.Check()
			} else {
				item.Uncheck()
			}
		}
	}
	a.mu.Unlock()
}

func (a *App) eventLoop() {
	for {
		select {
		case <-a.toggleItem.ClickedCh:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := a.controller.ToggleTunnel(ctx); err != nil {
				_ = beeep.Alert("Ошибка переключения", err.Error(), "")
			}
			cancel()
			a.updateUIState()

		case <-a.importItem.ClickedCh:
			a.handleImportClipboard()

		case <-a.updateGeoItem.ClickedCh:
			go a.handleUpdateGeoBases()

		case <-a.openRulesItem.ClickedCh:
			a.handleOpenRulesFolder()

		case <-a.quitItem.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *App) handleImportClipboard() {
	clip, err := getClipboardText()
	if err != nil || clip == "" {
		_ = beeep.Alert("Импорт профиля", "Буфер обмена пуст или недоступен", "")
		return
	}

	item, err := profile.ParseURI(clip)
	if err != nil {
		_ = beeep.Alert("Ошибка парсинга", fmt.Sprintf("Не удалось разобрать ссылку: %v", err), "")
		return
	}

	if err := a.profManager.Add(*item); err != nil {
		_ = beeep.Alert("Ошибка сохранения", err.Error(), "")
		return
	}

	// Перегенерируем конфиг и обновляем селектор демона
	if err := a.controller.ReloadDaemon(); err != nil {
		a.logger.Warn("could not reload daemon after import", slog.Any("error", err))
	}
	a.refreshProfileMenu()
	_ = beeep.Notify("Профиль добавлен", fmt.Sprintf("Добавлен узел: %s (проверен и готов к выбору)", item.Name), "")
}

func (a *App) handleUpdateGeoBases() {
	_ = beeep.Notify("Обновление баз", "Загрузка свежих правил GeoIP / GeoSite...", "")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := rules.UpdateGeoBases(ctx, a.paths.GeoIPPath, a.paths.GeoSitePath); err != nil {
		_ = beeep.Alert("Ошибка обновления", fmt.Sprintf("Сбой загрузки: %v", err), "")
		return
	}

	_ = beeep.Notify("Обновление завершено", "Базы geoip-ru и geosite-category-ru успешно обновлены!", "")
}

func (a *App) handleOpenRulesFolder() {
	dir := a.paths.BaseDir
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", dir).Start()
	case "windows":
		_ = exec.Command("explorer", dir).Start()
	default:
		_ = exec.Command("xdg-open", dir).Start()
	}
}

func getClipboardText() (string, error) {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("pbpaste").Output()
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("powershell", "-command", "Get-Clipboard").Output()
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	return "", fmt.Errorf("clipboard not supported")
}
