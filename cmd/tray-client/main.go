package main

import (
	"log/slog"
	"os"
	"path/filepath"

	"hysteria2-tray-client/internal/config"
	"hysteria2-tray-client/internal/core"
	"hysteria2-tray-client/internal/profile"
	"hysteria2-tray-client/internal/rules"
	"hysteria2-tray-client/internal/tray"
)

func main() {
	home, _ := os.UserHomeDir()
	logFile, err := os.OpenFile(filepath.Join(home, "Library/Logs/SingBoxTray.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	var logger *slog.Logger
	if err == nil {
		logger = slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	logger.Info("starting sing-box tray client...")

	paths, err := config.ResolvePaths()
	if err != nil {
		logger.Error("failed to resolve paths", slog.Any("error", err))
		os.Exit(1)
	}

	if err := rules.EnsureDefaultRuleFiles(paths.DirectDomains, paths.ProxyDomains); err != nil {
		logger.Warn("could not initialize default rule files", slog.Any("error", err))
	}

	profManager, err := profile.NewManager(paths.ProfilesDir)
	if err != nil {
		logger.Error("failed to initialize profile manager", slog.Any("error", err))
		os.Exit(1)
	}

	controller := core.NewController(logger, paths, profManager)

	app := tray.NewApp(logger, paths, profManager, controller)
	app.Run()
}
