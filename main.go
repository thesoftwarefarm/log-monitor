package main

import (
	"flag"
	"fmt"
	"os"

	"log-monitor/internal/config"
	"log-monitor/internal/logger"
	"log-monitor/internal/ui"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	debugLog := flag.String("debug", "", "path to debug log file (e.g. debug.log)")
	autoServer := flag.String("server", "", "auto-select server by name")
	autoFolder := flag.String("folder", "", "auto-select folder by path (requires -server)")
	autoFile := flag.String("file", "", "auto-select file by name (requires -server)")
	flag.Parse()

	if *debugLog != "" {
		if err := logger.Init(*debugLog); err != nil {
			fmt.Fprintf(os.Stderr, "Error opening debug log: %v\n", err)
			os.Exit(1)
		}
		defer logger.Close()
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	logger.Log("main", "config loaded, %d servers", len(cfg.Servers))

	app := ui.NewApp(cfg, ui.AutoSelect{
		Server: *autoServer,
		Folder: *autoFolder,
		File:   *autoFile,
	})
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	logger.Log("main", "app exited cleanly")
}
