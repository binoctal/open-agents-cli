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

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the bridge daemon",
	Long:  `Start the Open Agents bridge daemon in foreground mode.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Setup rotating logger
		l, err := logger.New()
		if err == nil {
			log.SetOutput(l.Writer())
			defer l.Close()
		}

		cfg, err := config.Load()
		if err != nil {
			log.Printf("Error loading config: %v", err)
			fmt.Println("Please run 'open-agents pair' first to configure the bridge.")
			os.Exit(1)
		}

		log.Println("Starting Open Agents Bridge...")
		log.Printf("Device ID: %s", cfg.DeviceID)
		log.Printf("Server: %s", cfg.ServerURL)

		b, err := bridge.New(cfg)
		if err != nil {
			log.Printf("Error creating bridge: %v", err)
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
			log.Println("Shutting down...")
			t.SetRunning(false)
			t.ShowNotification("Open Agents", "Bridge stopped")
			b.Stop()
		}()

		if err := b.Start(); err != nil {
			log.Printf("Bridge error: %v", err)
			os.Exit(1)
		}
	},
}
