package main

import (
	"fmt"
	"os"

	"github.com/open-agents/bridge/internal/config"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show bridge status",
	Long:  `Display the current status of the Open Agents bridge.`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Println("Status: Not configured")
			fmt.Println("Run 'open-agents pair' to configure the bridge.")
			os.Exit(0)
		}

		fmt.Println("Open Agents Bridge Status")
		fmt.Println("=========================")
		fmt.Printf("Device ID:    %s\n", cfg.DeviceID)
		fmt.Printf("User ID:      %s\n", cfg.UserID)
		fmt.Printf("Server:       %s\n", cfg.ServerURL)
		fmt.Println()

		// TODO: Check if bridge is running
		fmt.Println("Status: Configured (not running)")
		fmt.Println("Run 'open-agents start' to start the bridge.")
	},
}
