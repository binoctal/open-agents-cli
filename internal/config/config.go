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
	WebPubKey   string `json:"webPubKey,omitempty"`

	// v1.1: Device config synced from Web
	EnvVars     map[string]string `json:"envVars,omitempty"`
	CLIEnabled  map[string]bool   `json:"cliEnabled,omitempty"`
	Permissions map[string]bool   `json:"permissions,omitempty"`

	// v1.1: Auto-approval rules
	Rules []AutoApprovalRule `json:"rules,omitempty"`

	// v1.2: Storage settings
	StorageType string       `json:"storageType,omitempty"` // saas, s3, local
	S3Config    *S3Config    `json:"s3Config,omitempty"`
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
