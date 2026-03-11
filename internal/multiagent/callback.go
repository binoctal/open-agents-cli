package multiagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TaskResult encapsulates the result of a multi-agent task execution
type TaskResult struct {
	JobID      string `json:"jobId"`
	TaskID     string `json:"taskId"`
	Success    bool   `json:"success"`
	ExitCode   int    `json:"exitCode"`
	Summary    string `json:"summary"`    // Last 500 characters of output
	Artifacts  string `json:"artifacts"`  // Full output (limited to 100KB)
	Error      string `json:"error"`      // Error message if any
	ErrorType  string `json:"errorType"`  // "crash", "timeout", "cancelled"
	DurationMs int64  `json:"durationMs"`
}

// CallbackConfig holds configuration for the callback mechanism
type CallbackConfig struct {
	APIURL      string        // Base URL for the API
	DeviceID    string        // Device ID for identification
	Timeout     time.Duration // Task execution timeout (default 30min)
	MaxRetries  int           // Max retry attempts for callback (default 3)
	CacheDir    string        // Directory for caching failed callbacks
	MaxArtifactSize int       // Max artifact size in bytes (default 100KB)
}

// DefaultCallbackConfig returns default configuration
func DefaultCallbackConfig() CallbackConfig {
	homeDir, _ := os.UserHomeDir()
	return CallbackConfig{
		Timeout:         30 * time.Minute,
		MaxRetries:      3,
		CacheDir:        filepath.Join(homeDir, ".open-agents", "callback-cache"),
		MaxArtifactSize: 100 * 1024, // 100KB
	}
}

// CallbackManager handles task completion callbacks to the Orchestrator
type CallbackManager struct {
	config   CallbackConfig
	client   *http.Client
	cacheMu  sync.Mutex
}

// NewCallbackManager creates a new callback manager
func NewCallbackManager(config CallbackConfig) *CallbackManager {
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Minute
	}
	if config.MaxArtifactSize == 0 {
		config.MaxArtifactSize = 100 * 1024
	}

	// Ensure cache directory exists
	if config.CacheDir != "" {
		os.MkdirAll(config.CacheDir, 0755)
	}

	return &CallbackManager{
		config: config,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// ExtractArtifacts processes CLI output and extracts artifacts
func (m *CallbackManager) ExtractArtifacts(output []byte) (summary, artifacts string) {
	// Limit artifacts size
	artifactsBytes := output
	if len(artifactsBytes) > m.config.MaxArtifactSize {
		artifactsBytes = output[:m.config.MaxArtifactSize]
		artifacts = string(artifactsBytes) + "\n\n[... truncated ...]"
	} else {
		artifacts = string(artifactsBytes)
	}

	// Extract summary (last 500 characters)
	summaryBytes := output
	if len(summaryBytes) > 500 {
		summaryBytes = output[len(output)-500:]
	}
	summary = string(summaryBytes)

	return summary, artifacts
}

// SendTaskResult sends a task_result event to the Orchestrator
func (m *CallbackManager) SendTaskResult(result TaskResult) error {
	if m.config.APIURL == "" {
		log.Printf("[CallbackManager] No API URL configured, skipping callback")
		return nil
	}

	event := map[string]interface{}{
		"type": "multiagent:task_result",
		"payload": map[string]interface{}{
			"jobId":     result.JobID,
			"taskId":    result.TaskID,
			"artifacts": result.Artifacts,
			"summary":   result.Summary,
		},
		"timestamp": time.Now().UnixMilli(),
	}

	return m.sendEventWithRetry(event, result.TaskID)
}

// SendTaskError sends a task_error event to the Orchestrator
func (m *CallbackManager) SendTaskError(result TaskResult) error {
	if m.config.APIURL == "" {
		log.Printf("[CallbackManager] No API URL configured, skipping callback")
		return nil
	}

	event := map[string]interface{}{
		"type": "multiagent:task_error",
		"payload": map[string]interface{}{
			"jobId":      result.JobID,
			"taskId":     result.TaskID,
			"error":      result.Error,
			"errorType":  result.ErrorType,
		},
		"timestamp": time.Now().UnixMilli(),
	}

	return m.sendEventWithRetry(event, result.TaskID)
}

// sendEventWithRetry sends an event with exponential backoff retry
func (m *CallbackManager) sendEventWithRetry(event map[string]interface{}, taskID string) error {
	var lastErr error

	for attempt := 0; attempt < m.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 2s, 4s
			delay := time.Second * time.Duration(1<<uint(attempt-1))
			log.Printf("[CallbackManager] Retrying callback for task %s after %v (attempt %d/%d)",
				taskID, delay, attempt+1, m.config.MaxRetries)
			time.Sleep(delay)
		}

		err := m.postEvent(event)
		if err == nil {
			log.Printf("[CallbackManager] ✅ Callback successful for task %s", taskID)
			return nil
		}
		lastErr = err
		log.Printf("[CallbackManager] ⚠️ Callback failed for task %s: %v", taskID, err)
	}

	// All retries failed, cache for later
	if m.config.CacheDir != "" {
		if err := m.cacheEvent(event, taskID); err != nil {
			log.Printf("[CallbackManager] ❌ Failed to cache event for task %s: %v", taskID, err)
		} else {
			log.Printf("[CallbackManager] 📦 Cached event for task %s for later retry", taskID)
		}
	}

	return fmt.Errorf("callback failed after %d retries: %w", m.config.MaxRetries, lastErr)
}

// postEvent sends a single HTTP POST to the Orchestrator API
func (m *CallbackManager) postEvent(event map[string]interface{}) error {
	url := m.config.APIURL + "/api/multiagent/internal/orchestrator/event"

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Device-ID", m.config.DeviceID)

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// cacheEvent stores a failed event to disk for later retry
func (m *CallbackManager) cacheEvent(event map[string]interface{}, taskID string) error {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	filename := filepath.Join(m.config.CacheDir, taskID+".json")
	data, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// RetryCachedEvents attempts to send all cached events
func (m *CallbackManager) RetryCachedEvents() error {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	if m.config.CacheDir == "" {
		return nil
	}

	entries, err := os.ReadDir(m.config.CacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var failed int
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		taskID := entry.Name()[:len(entry.Name())-5]
		filename := filepath.Join(m.config.CacheDir, entry.Name())

		data, err := os.ReadFile(filename)
		if err != nil {
			log.Printf("[CallbackManager] Failed to read cached event %s: %v", filename, err)
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal(data, &event); err != nil {
			log.Printf("[CallbackManager] Failed to parse cached event %s: %v", filename, err)
			os.Remove(filename)
			continue
		}

		if err := m.postEvent(event); err != nil {
			log.Printf("[CallbackManager] Failed to send cached event for task %s: %v", taskID, err)
			failed++
			continue
		}

		// Success, remove cached file
		os.Remove(filename)
		log.Printf("[CallbackManager] ✅ Successfully sent cached event for task %s", taskID)
	}

	if failed > 0 {
		return fmt.Errorf("%d cached events failed to send", failed)
	}
	return nil
}

// GetTimeout returns the configured task timeout
func (m *CallbackManager) GetTimeout() time.Duration {
	return m.config.Timeout
}
