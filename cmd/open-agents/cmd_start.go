package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/open-agents/bridge/internal/bridge"
	"github.com/open-agents/bridge/internal/config"
	"github.com/open-agents/bridge/internal/logger"
	"github.com/open-agents/bridge/internal/tray"
	"github.com/spf13/cobra"
)

var (
	logLevel   string
	headless   bool
	deviceName string
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the bridge daemon",
	Long: `Start the Open Agents bridge daemon. This connects your
local CLI tools to the cloud and enables remote monitoring
and control.

Examples:
  # Start with current device
  open-agents start

  # Start a specific device
  open-agents start --device work-pc

  # Start with debug logging
  open-agents start --log-level debug`,
	Run: func(cmd *cobra.Command, args []string) {
		// Determine which device to use
		targetDevice := deviceName
		if targetDevice == "" {
			targetDevice = os.Getenv("OPEN_AGENTS_DEVICE")
		}

		// Setup rotating logger
		l, err := logger.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating logger: %v\n", err)
			os.Exit(1)
		}
		defer l.Close()

		// Redirect standard library log to custom logger
		log.SetOutput(l.Writer())
		log.SetFlags(0)

		// Set log level from flag
		logger.SetGlobalLevel(logLevel)

		var cfg *config.Config

		// Try multi-device config first
		if targetDevice != "" {
			cfg, err = config.LoadDevice(targetDevice)
			if err != nil {
				logger.Error("Error loading device config '%s': %v", targetDevice, err)
				fmt.Fprintf(os.Stderr, "Device '%s' not found. Run 'open-agents devices' to see available devices.\n", targetDevice)
				os.Exit(1)
			}
		} else {
			// Fall back to legacy single-device config
			cfg, err = config.Load()
			if err != nil {
				logger.Error("Error loading config: %v", err)
				fmt.Println("Please run 'open-agents pair' first to configure the bridge.")
				os.Exit(1)
			}
		}

		logger.Info("Starting Open Agents Bridge...")
		if cfg.DeviceName != "" {
			logger.Info("Device: %s", cfg.DeviceName)
		}
		logger.Info("Environment: %s", cfg.GetEnvironment())
		logger.Info("Device ID: %s", cfg.DeviceID)
		logger.Info("Server: %s", cfg.ServerURL)
		logger.Info("Log level: %s", logLevel)
		logger.Info("Log file: ~/.open-agents/logs/bridge-%s.log", time.Now().Format("2006-01-02"))
		logger.Info("E2EE: Keys loaded")

		b, err := bridge.New(cfg)
		if err != nil {
			logger.Error("Error creating bridge: %v", err)
			os.Exit(1)
		}

		// Setup system tray notification
		trayTitle := "Open Agents"
		if cfg.DeviceName != "" {
			trayTitle = fmt.Sprintf("Open Agents (%s)", cfg.DeviceName)
		}
		t := tray.New(trayTitle)
		t.SetRunning(true)
		deviceDisplay := cfg.DeviceName
		if deviceDisplay == "" {
			deviceDisplay = "default"
		}
		t.ShowNotification("Open Agents", fmt.Sprintf("Bridge started (%s)", deviceDisplay))

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		go func() {
			<-sigChan
			logger.Info("Shutting down...")
			t.SetRunning(false)
			t.ShowNotification("Open Agents", "Bridge stopped")
			b.Stop()
		}()

		if err := b.Start(); err != nil {
			logger.Error("Bridge error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	startCmd.Flags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (error, warn, info, debug)")
	startCmd.Flags().BoolVarP(&headless, "headless", "H", false, "Run in headless mode (no system tray)")
	startCmd.Flags().StringVarP(&deviceName, "device", "d", "", "Device name to start (default: current device)")
}
