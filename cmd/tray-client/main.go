package main

import (
	"log/slog"
	"os"

	"hysteria2-tray-client/internal/config"
	"hysteria2-tray-client/internal/core"
	"hysteria2-tray-client/internal/profile"
	"hysteria2-tray-client/internal/rules"
	"hysteria2-tray-client/internal/tray"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("starting sing-box tray client...")

	paths, err := config.ResolvePaths()
	if err != nil {
		logger.Error("failed to resolve paths", slog.Any("error", err))
		os.Exit(1)
	}

	// Инициализируем файлы правил исключений (direct_domains.txt, proxy_domains.txt)
	if err := rules.EnsureDefaultRuleFiles(paths.DirectDomains, paths.ProxyDomains); err != nil {
		logger.Warn("could not initialize default rule files", slog.Any("error", err))
	}

	// Инициализируем менеджер профилей
	profManager, err := profile.NewManager(paths.ProfilesDir)
	if err != nil {
		logger.Error("failed to initialize profile manager", slog.Any("error", err))
		os.Exit(1)
	}

	controller := core.NewController(logger, paths, profManager)

	// Запуск графического трей-приложения
	app := tray.NewApp(logger, paths, profManager, controller)
	app.Run()
}
