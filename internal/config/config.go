package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

type Config struct {
	UserID      string `json:"userId"`
	DeviceID    string `json:"deviceId"`
	DeviceToken string `json:"deviceToken"`
	ServerURL   string `json:"serverUrl"`
	PublicKey   string `json:"publicKey,omitempty"`
	PrivateKey  string `json:"privateKey,omitempty"`
	WebPubKey   string `json:"webPubKey,omitempty"` // Web client's public key
}

// ConfigDir returns the configuration directory path
func ConfigDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "open-agents")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".open-agents")
	}
}

// ConfigPath returns the full path to the config file
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

// Load reads the configuration from disk
func Load() (*Config, error) {
	data, err := os.ReadFile(ConfigPath())
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save writes the configuration to disk
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
