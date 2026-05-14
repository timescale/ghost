package util

import (
	"os"
	"path/filepath"
	"strings"
)

// DisplayPath replaces $HOME with ~ in path for compact display.
func DisplayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

// ExpandPath expands environment variables and tilde in file paths.
// It handles:
// - Empty paths (returns empty string)
// - Environment variable expansion (e.g., $HOME/config)
// - Home directory expansion (e.g., ~/config or ~)
// - Path normalization for cross-platform compatibility
func ExpandPath(path string) string {
	if path == "" {
		return ""
	}

	// Expand environment variables
	expanded := os.ExpandEnv(path)

	// Expand home directory
	if expanded == "~" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			return homeDir
		}
		// If UserHomeDir fails, fall through to return original
	}

	if strings.HasPrefix(expanded, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			expanded = filepath.Join(homeDir, expanded[2:])
		}
		// If UserHomeDir fails, leave expanded as-is (with ~/ prefix)
	}

	return filepath.Clean(expanded)
}
