package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/timescale/ghost/internal/api"
	"github.com/timescale/ghost/internal/config"
	"golang.org/x/mod/semver"
)

// InstallMethod represents how Ghost CLI was installed
type InstallMethod string

const (
	InstallMethodHomebrew   InstallMethod = "homebrew"
	InstallMethodDeb        InstallMethod = "deb"
	InstallMethodRPM        InstallMethod = "rpm"
	InstallMethodInstallSh  InstallMethod = "install_sh"
	InstallMethodInstallPS1 InstallMethod = "install_ps1"
	InstallMethodUnknown    InstallMethod = "unknown"
)

// VersionCheckResult contains the result of a version check
type VersionCheckResult struct {
	UpdateAvailable bool
	LatestVersion   string
	CurrentVersion  string
	InstallMethod   InstallMethod
	UpdateCommand   string
}

// CheckVersion checks if a new version is available and returns the result.
func CheckVersion(ctx context.Context, releasesURL string) (*VersionCheckResult, error) {
	latestVersion, err := fetchLatestVersion(ctx, releasesURL)
	if err != nil {
		return nil, err
	}

	updateAvailable, err := compareVersions(config.Version, latestVersion)
	if err != nil {
		return nil, err
	}

	installMethod := detectInstallMethod(ctx)
	updateCommand := getUpdateCommand(installMethod)

	return &VersionCheckResult{
		UpdateAvailable: updateAvailable,
		LatestVersion:   latestVersion,
		CurrentVersion:  config.Version,
		InstallMethod:   installMethod,
		UpdateCommand:   updateCommand,
	}, nil
}

// fetchLatestVersion downloads the latest version string from the given URL
func fetchLatestVersion(ctx context.Context, releasesURL string) (string, error) {
	latestURL := releasesURL + "/latest.txt"

	// Create a context with 5s timeout (the default request timeout is 30s but
	// we don't want to wait that long for these requests).
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := api.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest version: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	version := strings.TrimSpace(string(body))
	if version == "" {
		return "", errors.New("empty version string in response")
	}

	return version, nil
}

// compareVersions returns true if newVersion is greater than currentVersion
func compareVersions(currentVersion, newVersion string) (bool, error) {
	// Handle development version
	if currentVersion == "dev" {
		return false, nil
	}

	// Validate both versions
	if !semver.IsValid(currentVersion) {
		return false, fmt.Errorf("invalid semver version: %s", currentVersion)
	}

	if !semver.IsValid(newVersion) {
		return false, fmt.Errorf("invalid semver version: %s", newVersion)
	}

	// Compare returns 1 if newVersion > currentVersion
	return semver.Compare(newVersion, currentVersion) > 0, nil
}

// detectInstallMethod determines how Ghost CLI was installed
func detectInstallMethod(ctx context.Context) InstallMethod {
	// Detect installation method
	binaryPath, err := os.Executable()
	if err != nil {
		return InstallMethodUnknown
	}

	// Check for Homebrew installation
	lowerPath := strings.ToLower(binaryPath)
	if isUnderHomebrew(ctx, binaryPath) || strings.Contains(lowerPath, "/homebrew/") || strings.Contains(lowerPath, "/linuxbrew/") {
		return InstallMethodHomebrew
	}

	// Check if installed via dpkg (Debian/Ubuntu)
	if runtime.GOOS == "linux" {
		if output, err := exec.CommandContext(ctx, "dpkg", "-S", binaryPath).CombinedOutput(); err == nil {
			if strings.Contains(string(output), "ghost") {
				return InstallMethodDeb
			}
		}

		// Check if installed via rpm (RHEL/Fedora/CentOS)
		if output, err := exec.CommandContext(ctx, "rpm", "-qf", binaryPath).CombinedOutput(); err == nil {
			if strings.Contains(string(output), "ghost") {
				return InstallMethodRPM
			}
		}
	}

	// Check if installed via install.ps1 (Windows: typically in LOCALAPPDATA\Programs\Ghost)
	if runtime.GOOS == "windows" {
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			defaultDir := filepath.Join(localAppData, "Programs", "Ghost") + string(filepath.Separator)
			if strings.HasPrefix(strings.ToLower(binaryPath), strings.ToLower(defaultDir)) {
				return InstallMethodInstallPS1
			}
		}
	}

	// Check if installed via install.sh (typically in ~/.local/bin or ~/bin)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		localBin := filepath.Join(homeDir, ".local", "bin") + string(filepath.Separator)
		homeBin := filepath.Join(homeDir, "bin") + string(filepath.Separator)

		if strings.HasPrefix(binaryPath, localBin) || strings.HasPrefix(binaryPath, homeBin) {
			return InstallMethodInstallSh
		}
	}

	return InstallMethodUnknown
}

// borrowed from GH cli
// https://github.com/cli/cli/blob/trunk/internal/ghcmd/cmd.go#L233
func isUnderHomebrew(ctx context.Context, binaryPath string) bool {
	brewExe, err := exec.LookPath("brew")
	if err != nil {
		return false
	}

	brewPrefixBytes, err := exec.CommandContext(ctx, brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)
	return strings.HasPrefix(binaryPath, brewBinPrefix)
}

// getUpdateCommand returns the command to update Ghost CLI based on the install method
func getUpdateCommand(method InstallMethod) string {
	switch method {
	case InstallMethodHomebrew:
		return "brew update && brew upgrade ghost"
	case InstallMethodDeb:
		return "sudo apt update && sudo apt install ghost"
	case InstallMethodRPM:
		// Try to detect which package manager is available
		if _, err := exec.LookPath("dnf"); err == nil {
			return "sudo dnf update ghost"
		}
		return "sudo yum update ghost"
	case InstallMethodInstallSh, InstallMethodInstallPS1, InstallMethodUnknown:
		// `ghost upgrade` replaces the binary in place; if it can't (e.g.
		// wrong permissions or an unrecognized package manager), it reports
		// a clear error directing the user back to their original install
		// method.
		return "ghost upgrade"
	default:
		return ""
	}
}
