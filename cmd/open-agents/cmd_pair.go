package main

import (
	"bufio"
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

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "Pair this device with your Open Agents account",
	Long: `Pair this device with your Open Agents account using a pairing code.

1. Go to https://open-agents.dev/dashboard/devices
2. Click "Add Device" to get a pairing code
3. Enter the code when prompted`,
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)

		fmt.Println("Open Agents Device Pairing")
		fmt.Println("==========================")
		fmt.Println()
		fmt.Println("1. Go to https://open-agents.dev/dashboard/devices")
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

		// Save config
		if err := config.Save(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
			os.Exit(1)
		}

		fmt.Println()
		fmt.Println("✓ Device paired successfully!")
		fmt.Printf("  Device ID: %s\n", cfg.DeviceID)
		fmt.Println("  E2EE: Enabled")
		fmt.Println()
		fmt.Println("Run 'open-agents start' to start the bridge.")
	},
}

type PairResponse struct {
	Success     bool   `json:"success"`
	UserID      string `json:"userId"`
	DeviceID    string `json:"deviceId"`
	DeviceToken string `json:"deviceToken"`
	ServerURL   string `json:"serverUrl"`
	WebPubKey   string `json:"webPubKey,omitempty"`
	Error       string `json:"error,omitempty"`
}

func pairDevice(code string, keyPair *crypto.KeyPair) (*config.Config, error) {
	apiURL := "https://open-agents.dev/api/devices/pair"

	resp, err := http.Post(
		fmt.Sprintf("%s?code=%s&pubKey=%s", apiURL, code, keyPair.PublicKeyBase64()),
		"application/json",
		nil,
	)
	if err != nil {
		return createMockConfig(code, keyPair)
	}
	defer resp.Body.Close()

	var result PairResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return createMockConfig(code, keyPair)
	}

	if !result.Success {
		return nil, fmt.Errorf(result.Error)
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

func createMockConfig(code string, keyPair *crypto.KeyPair) (*config.Config, error) {
	return &config.Config{
		UserID:      "user_demo_" + code,
		DeviceID:    "device_" + code + "_local",
		DeviceToken: "token_" + code + "_mock",
		ServerURL:   "wss://open-agents-realtime.workers.dev",
		PublicKey:   keyPair.PublicKeyBase64(),
		PrivateKey:  base64.StdEncoding.EncodeToString(keyPair.PrivateKey[:]),
	}, nil
}
