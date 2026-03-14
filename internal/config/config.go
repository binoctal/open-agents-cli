package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// DeviceConfig represents a single device configuration
type DeviceConfig struct {
	Name        string    `json:"name"`
	UserID      string    `json:"userId"`
	DeviceID    string    `json:"deviceId"`
	DeviceToken string    `json:"deviceToken"`
	ServerURL   string    `json:"serverUrl"`
	PublicKey   string    `json:"publicKey,omitempty"`
	PrivateKey  string    `json:"privateKey,omitempty"`
	WebPubKey   string    `json:"webPubKey,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	LastUsedAt  time.Time `json:"lastUsedAt,omitempty"`

	// Device-specific settings (override global)
	CLIEnabled  map[string]bool `json:"cliEnabled,omitempty"`
	Permissions map[string]bool `json:"permissions,omitempty"`
	LogLevel    string          `json:"logLevel,omitempty"`
}

// GlobalConfig represents the global configuration
type GlobalConfig struct {
	CurrentDevice   string                     `json:"currentDevice"`
	DefaultServerURL string                    `json:"defaultServerUrl"`
	CLIEnabled      map[string]bool            `json:"cliEnabled,omitempty"`
	Permissions     map[string]bool            `json:"permissions,omitempty"`
	LogLevel        string                     `json:"logLevel,omitempty"`
}

// Config is the legacy single-device config (for backward compatibility)
type Config struct {
	UserID      string `json:"userId"`
	DeviceID    string `json:"deviceId"`
	DeviceToken string `json:"deviceToken"`
	ServerURL   string `json:"serverUrl"`
	PublicKey   string `json:"publicKey,omitempty"`
	PrivateKey  string `json:"privateKey,omitempty"`
	WebPubKey   string `json:"webPubKey,omitempty"`

	// v1.1: Device config synced from Web
	EnvVars     map[string]string `json:"envVars,omitempty"`
	CLIEnabled  map[string]bool   `json:"cliEnabled,omitempty"`
	Permissions map[string]bool   `json:"permissions,omitempty"`

	// v1.1: Auto-approval rules
	Rules []AutoApprovalRule `json:"rules,omitempty"`

	// v1.2: Storage settings
	StorageType string    `json:"storageType,omitempty"` // saas, s3, local
	S3Config    *S3Config `json:"s3Config,omitempty"`

	// v1.3: Logging settings
	LogLevel string `json:"logLevel,omitempty"` // debug, info, warn, error (default: info)

	// v1.4: Synced prompts from Web
	Prompts interface{} `json:"prompts,omitempty"`

	// v2.2: Model fallback chain
	ModelFallbacks []ModelFallback `json:"modelFallbacks,omitempty"`

	// v2.3: Security scanner
	ScannerEnabled *bool `json:"scannerEnabled,omitempty"` // nil = default (true)

	// v2.4: Environment setting (optional, auto-detected if not set)
	// Values: development, staging, production
	Environment string `json:"environment,omitempty"`

	// v2.5: Device name (for multi-device support)
	DeviceName string `json:"deviceName,omitempty"`
}

// GetEnvironment returns the environment setting.
// Priority: 1. Explicit Environment field in config
//
//  2. Auto-detect from ServerURL
func (c *Config) GetEnvironment() string {
	// If explicitly set in config, use that
	if c.Environment != "" {
		return c.Environment
	}

	// Auto-detect from ServerURL
	if c.ServerURL == "" {
		return "unknown"
	}
	// Check for staging indicators
	if strings.Contains(c.ServerURL, "staging") ||
		strings.Contains(c.ServerURL, "preview") ||
		strings.Contains(c.ServerURL, "-staging") {
		return "staging"
	}
	// Check for localhost
	if strings.Contains(c.ServerURL, "localhost") ||
		strings.Contains(c.ServerURL, "127.0.0.1") {
		return "development"
	}
	// Default to production
	return "production"
}

type ModelFallback struct {
	CLIType  string `json:"cliType"`            // which CLI this applies to
	Fallback string `json:"fallback"`            // fallback CLI to use
	OnError  string `json:"onError,omitempty"`   // "rate_limit", "timeout", "any" (default: "any")
}

type AutoApprovalRule struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
	Tool    string `json:"tool"`
	Action  string `json:"action"` // auto-approve, ask, deny
}

type S3Config struct {
	Bucket    string `json:"bucket"`
	Region    string `json:"region"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
	Endpoint  string `json:"endpoint,omitempty"`
}

func ConfigDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "open-agents")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".open-agents")
	}
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func Load() (*Config, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Initialize maps if nil
	if cfg.EnvVars == nil {
		cfg.EnvVars = make(map[string]string)
	}
	if cfg.CLIEnabled == nil {
		cfg.CLIEnabled = map[string]bool{"kiro": true, "claude": true, "cline": true, "codex": true, "gemini": true}
	}
	if cfg.Permissions == nil {
		cfg.Permissions = map[string]bool{"fs_read": true, "fs_write": true, "execute_bash": true, "network": false}
	}

	return &cfg, nil
}

func Save(cfg *Config) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ConfigPath(), data, 0600)
}

// SaveScannerRules persists custom scanner rules to a separate file.
func SaveScannerRules(rules interface{}) error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	wrapper := map[string]interface{}{"customRules": rules}
	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	 }
	return os.WriteFile(filepath.Join(dir, "scanner-rules.json"), data, 0600)
}

// ============================================
// Multi-Device Support
// ============================================

// DevicesDir returns the directory for device-specific configs
func DevicesDir() string {
	return filepath.Join(ConfigDir(), "devices")
}

// DeviceConfigPath returns the path to a device's config file
func DeviceConfigPath(name string) string {
	return filepath.Join(DevicesDir(), name+".json")
}

// ListDevices returns all configured device names
func ListDevices() ([]string, error) {
	dir := DevicesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
		return []string{}, nil
		}
		return nil, err
	}

	var devices []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			name := strings.TrimSuffix(entry.Name(), ".json")
			devices = append(devices, name)
		}
	}
	return devices, nil
}

// LoadDevice loads a specific device's config
func LoadDevice(name string) (*Config, error) {
	path := DeviceConfigPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set device name if not set
	if cfg.DeviceName == "" {
		cfg.DeviceName = name
	}

	// Initialize maps if nil
	if cfg.EnvVars == nil {
		cfg.EnvVars = make(map[string]string)
	}
	if cfg.CLIEnabled == nil {
		cfg.CLIEnabled = map[string]bool{"kiro": true, "claude": true, "cline": true, "codex": true, "gemini": true}
	}
	if cfg.Permissions == nil {
		cfg.Permissions = map[string]bool{"fs_read": true, "fs_write": true, "execute_bash": true, "network": false}
	}

	return &cfg, nil
}

// SaveDevice saves a device's config
func SaveDevice(name string, cfg *Config) error {
	dir := DevicesDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	// Ensure device name is set
	cfg.DeviceName = name

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(DeviceConfigPath(name), data, 0600)
}

// DeleteDevice removes a device's config
func DeleteDevice(name string) error {
	return os.Remove(DeviceConfigPath(name))
}

// GetCurrentDevice returns the current device name (from global config or env)
func GetCurrentDevice() string {
	// Check environment variable first
	if name := os.Getenv("OPEN_AGENTS_DEVICE"); name != "" {
		return name
	}

	// If only one device exists, use that
	devices, _ := ListDevices()
	if len(devices) == 1 {
		return devices[0]
	}

	// Return empty if no current device set
	return ""
}

// SetCurrentDevice sets the current device in global config
func SetCurrentDevice(name string) error {
	_ = name // For now, we use environment variable approach
	// The caller should set OPEN_AGENTS_DEVICE env var
	return nil
}

// DeviceExists checks if a device config exists
func DeviceExists(name string) bool {
	_, err := os.Stat(DeviceConfigPath(name))
	return err == nil
}
