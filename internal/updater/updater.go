package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	RepoOwner = "open-agents"
	RepoName  = "open-agents"
	Version   = "0.1.0"
)

// Release represents a GitHub release
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset
type Asset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

// CheckUpdate checks for a new version
func CheckUpdate() (*Release, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", RepoOwner, RepoName)

	resp, err := http.Get(url)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, false, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, false, err
	}

	// Compare versions (simple string comparison)
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	hasUpdate := latestVersion > Version

	return &release, hasUpdate, nil
}

// GetAssetForPlatform returns the download URL for current platform
func GetAssetForPlatform(release *Release) string {
	suffix := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		suffix += ".exe"
	}

	for _, asset := range release.Assets {
		if strings.Contains(asset.Name, suffix) {
			return asset.DownloadURL
		}
	}
	return ""
}

// DownloadUpdate downloads the new binary
func DownloadUpdate(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "open-agents-update-*")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// ApplyUpdate replaces the current binary with the new one
func ApplyUpdate(newBinary string) error {
	currentPath, err := os.Executable()
	if err != nil {
		return err
	}
	currentPath, _ = filepath.Abs(currentPath)

	// Backup current binary
	backupPath := currentPath + ".bak"
	os.Remove(backupPath)
	if err := os.Rename(currentPath, backupPath); err != nil {
		return err
	}

	// Move new binary
	if err := os.Rename(newBinary, currentPath); err != nil {
		// Restore backup on failure
		os.Rename(backupPath, currentPath)
		return err
	}

	// Make executable
	os.Chmod(currentPath, 0755)

	// Remove backup
	os.Remove(backupPath)

	return nil
}
