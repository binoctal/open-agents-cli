package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/open-agents/bridge/internal/bridge"
	"github.com/open-agents/bridge/internal/config"
	"github.com/open-agents/bridge/internal/logger"
	"github.com/open-agents/bridge/internal/tray"
	"github.com/spf13/cobra"
)

var (
	logLevel string
	headless bool
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the bridge daemon",
	Long:  `Start the Open Agents bridge daemon. This connects your
local CLI tools to the cloud and enables remote monitoring
and control.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Setup rotating logger
		l, err := logger.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating logger: %v\n", err)
			os.Exit(1)
		}
		defer l.Close()

		// Redirect standard library log to custom logger
		// This ensures all log.Printf calls use our log level and file output
		log.SetOutput(l.Writer())
		log.SetFlags(0) // Custom logger already adds timestamp

		// Set log level from flag
		logger.SetGlobalLevel(logLevel)

		cfg, err := config.Load()
		if err != nil {
				logger.Error("Error loading config: %v", err)
				fmt.Println("Please run 'open-agents pair' first to configure the bridge.")
				os.Exit(1)
		}

		logger.Info("Starting Open Agents Bridge...")
		logger.Info("Device ID: %s", cfg.DeviceID)
		logger.Info("Server: %s", cfg.ServerURL)
		logger.Info("Log level: %s", logLevel)

		b, err := bridge.New(cfg)
		if err != nil {
				logger.Error("Error creating bridge: %v", err)
				os.Exit(1)
		}

		// Setup system tray notification
		t := tray.New("Open Agents Bridge")
		t.SetRunning(true)
		t.ShowNotification("Open Agents", "Bridge started successfully")

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
}
