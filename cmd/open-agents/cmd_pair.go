package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/open-agents/bridge/internal/config"
	"github.com/open-agents/bridge/internal/crypto"
	"github.com/spf13/cobra"
)

var pairServerURL string

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "Pair this device with your Open Agents account",
	Long: `Pair this device with your Open Agents account using a pairing code.

1. Go to your server's dashboard/devices page
2. Click "Add Device" to get a pairing code
3. Enter the code when prompted

Examples:
  # Local development
  open-agents pair --server http://localhost:8787

  # Custom server
  open-agents pair --server https://your-server.com`,
	Run: func(cmd *cobra.Command, args []string) {
		// Server URL is required
		if pairServerURL == "" {
			fmt.Fprintln(os.Stderr, "Error: --server flag is required")
			fmt.Fprintln(os.Stderr, "Example: open-agents pair --server http://localhost:8787")
			os.Exit(1)
		}

		reader := bufio.NewReader(os.Stdin)

		fmt.Println("Open Agents Device Pairing")
		fmt.Println("==========================")
		fmt.Println()
		fmt.Printf("Using server: %s\n", pairServerURL)

		// Determine dashboard URL based on server
		dashboardURL := strings.TrimSuffix(pairServerURL, "/") + "/dashboard/devices"

		fmt.Printf("1. Go to %s\n", dashboardURL)
		fmt.Println("2. Click 'Add Device' to get a pairing code")
		fmt.Println()
		fmt.Print("Enter pairing code: ")

		code, _ := reader.ReadString('\n')
		code = strings.TrimSpace(code)

		if len(code) != 6 {
			fmt.Println("Error: Pairing code must be 6 digits")
			os.Exit(1)
		}

		fmt.Println("Generating encryption keys...")

		// Generate E2EE key pair
		keyPair, err := crypto.GenerateKeyPair()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating keys: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Pairing...")

		// Call pairing API
		cfg, err := pairDevice(code, keyPair)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Pairing failed: %v\n", err)
			os.Exit(1)
		}

		// Convert http(s) to ws(s)
		wsURL := strings.Replace(pairServerURL, "http://", "ws://", 1)
		wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
		cfg.ServerURL = wsURL

		// Save config
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Println("✓ Device paired successfully!")
		fmt.Printf("  Device ID: %s\n", cfg.DeviceID)
		fmt.Printf("  Server: %s\n", cfg.ServerURL)
		fmt.Println("  E2EE: Enabled")
		fmt.Println()
		fmt.Println("Run 'open-agents start' to start the bridge.")
	},
}

func init() {
	pairCmd.Flags().StringVarP(&pairServerURL, "server", "s", "", "Server URL (required, e.g., http://localhost:8787)")
}

type PairResponse struct {
	Success     bool          `json:"success"`
	UserID      string        `json:"userId"`
	DeviceID    string        `json:"deviceId"`
	DeviceToken string        `json:"deviceToken"`
	ServerURL   string        `json:"serverUrl"`
	WebPubKey   string        `json:"webPubKey,omitempty"`
	Error       *ErrorResponse `json:"error,omitempty"`
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func pairDevice(code string, keyPair *crypto.KeyPair) (*config.Config, error) {
	apiURL := strings.TrimSuffix(pairServerURL, "/") + "/api/devices/pair/verify"

	body := map[string]string{
		"pairCode": code,
	}
	bodyJSON, _ := json.Marshal(body)

	resp, err := http.Post(
		apiURL,
		"application/json",
		bytes.NewBuffer(bodyJSON),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	var result PairResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !result.Success && result.Error != nil {
		return nil, fmt.Errorf("%s: %s", result.Error.Code, result.Error.Message)
	}

	if !result.Success {
		return nil, fmt.Errorf("pairing failed")
	}

	return &config.Config{
		UserID:      result.UserID,
		DeviceID:    result.DeviceID,
		DeviceToken: result.DeviceToken,
		ServerURL:   result.ServerURL,
		PublicKey:   keyPair.PublicKeyBase64(),
		PrivateKey:  base64.StdEncoding.EncodeToString(keyPair.PrivateKey[:]),
		WebPubKey:   result.WebPubKey,
	}, nil
}
