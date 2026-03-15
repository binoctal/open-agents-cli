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

var (
	pairServerURL   string
	pairAutoStart   bool
	pairDevMode     bool
	pairDevEmail    string
	pairDevPassword string
)

// Default server URLs
const (
	defaultAPIURL = "https://api.openagents.top"
	defaultWebURL = "https://openagents.top"
)

var pairCmd = &cobra.Command{
	Use:   "pair",
	Short: "Pair this device with your Open Agents account",
	Long: `Pair this device with your Open Agents account using a pairing code.

1. Go to the dashboard at https://open-agents-web.pages.dev/dashboard/devices
2. Click "Add Device" to get a pairing code
3. Enter the code when prompted

Examples:
  # Use default production server
  open-agents pair

  # Local development
  open-agents pair --server http://localhost:8787

  # Dev mode: auto-create test user and device (localhost only)
  open-agents pair --dev --server http://localhost:8787`,
	Run: func(cmd *cobra.Command, args []string) {
		// Use default production API URL if not specified
		if pairServerURL == "" {
			pairServerURL = defaultAPIURL
		}

		// Handle --dev mode
		if pairDevMode {
			runDevPair(cmd, args)
			return
		}

		reader := bufio.NewReader(os.Stdin)

		fmt.Println("Open Agents Device Pairing")
		fmt.Println("==========================")
		fmt.Println()
		fmt.Printf("Using API server: %s\n", pairServerURL)

		// Determine dashboard URL based on server
		var dashboardURL string
		if pairServerURL == defaultAPIURL {
			dashboardURL = defaultWebURL + "/dashboard/devices"
		} else if strings.Contains(pairServerURL, "localhost") {
			dashboardURL = "http://localhost:5173/dashboard/devices"
		} else {
			dashboardURL = strings.TrimSuffix(pairServerURL, "/") + "/dashboard/devices"
		}

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

		// Auto-start if requested
		if pairAutoStart {
			fmt.Println("Starting bridge automatically...")
			fmt.Println()
			startCmd.Run(cmd, args)
		} else {
			fmt.Println("Run 'open-agents start' to start the bridge.")
		}
	},
}

// runDevPair handles --dev mode for quick local development setup
func runDevPair(cmd *cobra.Command, args []string) {
	// Safety check: only allow dev mode with localhost
	if !strings.Contains(pairServerURL, "localhost") && !strings.Contains(pairServerURL, "127.0.0.1") {
		fmt.Fprintln(os.Stderr, "Error: --dev mode is only allowed with localhost servers")
		fmt.Fprintln(os.Stderr, "Use: open-agents pair --dev --server http://localhost:8787")
		os.Exit(1)
	}

	fmt.Println("Open Agents Dev Setup")
	fmt.Println("=====================")
	fmt.Println()
	fmt.Printf("Using API server: %s\n", pairServerURL)

	// Set defaults
	email := pairDevEmail
	if email == "" {
		email = "dev@openagents.local"
	}
	password := pairDevPassword
	if password == "" {
		password = "dev123456"
	}

	fmt.Printf("Setting up device for: %s\n", email)

	// Call dev setup API
	cfg, err := devSetup(email, password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Dev setup failed: %v\n", err)
		os.Exit(1)
	}

	// Convert http(s) to ws(s)
	wsURL := strings.Replace(pairServerURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	cfg.ServerURL = wsURL

	// Generate encryption keys
	fmt.Println("Generating encryption keys...")
	keyPair, err := crypto.GenerateKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating keys: %v\n", err)
		os.Exit(1)
	}
	cfg.PublicKey = keyPair.PublicKeyBase64()
	cfg.PrivateKey = base64.StdEncoding.EncodeToString(keyPair.PrivateKey[:])

	// Save config
	if err := config.Save(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✓ Dev environment ready!")
	fmt.Printf("  User: %s\n", email)
	fmt.Printf("  Device ID: %s\n", cfg.DeviceID)
	fmt.Printf("  Server: %s\n", cfg.ServerURL)
	fmt.Println()

	// Auto-start if requested
	if pairAutoStart {
		fmt.Println("Starting bridge automatically...")
		fmt.Println()
		startCmd.Run(cmd, args)
	} else {
		fmt.Println("Run 'open-agents start' to start the bridge.")
	}
}

func init() {
	pairCmd.Flags().StringVarP(&pairServerURL, "server", "s", "", "API server URL (default: production server)")
	pairCmd.Flags().BoolVarP(&pairAutoStart, "auto-start", "a", false, "Automatically start bridge after pairing")
	pairCmd.Flags().BoolVarP(&pairDevMode, "dev", "d", false, "Development mode: auto-create test user and device (localhost only)")
	pairCmd.Flags().StringVar(&pairDevEmail, "email", "", "Dev mode: custom email (default: dev@openagents.local)")
	pairCmd.Flags().StringVar(&pairDevPassword, "password", "", "Dev mode: custom password (default: dev123456)")
}

type PairResponse struct {
	Success     bool           `json:"success"`
	UserID      string         `json:"userId"`
	DeviceID    string         `json:"deviceId"`
	DeviceToken string         `json:"deviceToken"`
	ServerURL   string         `json:"serverUrl"`
	WebPubKey   string         `json:"webPubKey,omitempty"`
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

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")

	client := &http.Client{}
	resp, err := client.Do(req)
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

// DevSetupResponse represents the response from /api/dev/setup
type DevSetupResponse struct {
	Success bool `json:"success"`
	User    struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		Password string `json:"password"`
	} `json:"user"`
	Device struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Token string `json:"token"`
	} `json:"device"`
	Config struct {
		UserID      string `json:"userId"`
		DeviceID    string `json:"deviceId"`
		DeviceToken string `json:"deviceToken"`
		ServerURL   string `json:"serverUrl"`
	} `json:"config"`
	Error *ErrorResponse `json:"error,omitempty"`
}

// devSetup calls the /api/dev/setup endpoint for quick local development
func devSetup(email, password string) (*config.Config, error) {
	apiURL := strings.TrimSuffix(pairServerURL, "/") + "/api/dev/setup"

	body := map[string]string{
		"email":    email,
		"password": password,
	}
	bodyJSON, _ := json.Marshal(body)

	// Create request with Origin header for CSRF bypass (dev mode only)
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	var result DevSetupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !result.Success && result.Error != nil {
		return nil, fmt.Errorf("%s: %s", result.Error.Code, result.Error.Message)
	}

	if !result.Success {
		return nil, fmt.Errorf("dev setup failed")
	}

	return &config.Config{
		UserID:      result.Config.UserID,
		DeviceID:    result.Config.DeviceID,
		DeviceToken: result.Config.DeviceToken,
	}, nil
}
