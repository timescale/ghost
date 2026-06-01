package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
	"golang.org/x/oauth2"
)

// Keyring parameters
const (
	keyringServiceName = "ghost-cli"
	keyringUsername    = "credentials"
)

// Credentials represents the JSON structure for stored credentials.
// Legacy logins populate APIKey; new OAuth logins populate Token.
type Credentials struct {
	APIKey    string        `json:"api_key,omitempty"`
	ProjectID string        `json:"project_id"`
	Token     *oauth2.Token `json:"token,omitempty"`
}

func (c *Config) credentialsFileName() string {
	return fmt.Sprintf("%s/credentials", c.ConfigDir)
}

// StoreCredentials stores credentials (either an OAuth token or API key, plus project ID).
func (c *Config) StoreCredentials(creds Credentials) error {
	credentialsJSON, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Try keyring first, unless disabled
	if c.Keyring {
		if err := c.storeToKeyring(string(credentialsJSON)); err == nil {
			return nil
		}
	}

	// Fallback to file storage
	return c.storeToFile(string(credentialsJSON))
}

func (c *Config) storeToKeyring(credentials string) error {
	return keyring.Set(keyringServiceName, keyringUsername, credentials)
}

// storeToFile stores credentials to ~/.config/ghost/credentials with restricted permissions
func (c *Config) storeToFile(credentials string) error {
	credentialsFile := c.credentialsFileName()
	if err := os.MkdirAll(filepath.Dir(credentialsFile), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	file, err := os.OpenFile(credentialsFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create credentials file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(credentials); err != nil {
		return fmt.Errorf("failed to write credentials to file: %w", err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	return nil
}

var ErrNotLoggedIn = errors.New("not logged in")

// GetCredentials retrieves stored credentials (OAuth token or API key + project ID).
func (c *Config) GetCredentials() (Credentials, error) {
	// Try keyring first, unless disabled
	if c.Keyring {
		if creds, err := c.getCredentialsFromKeyring(); err == nil {
			return creds, nil
		}
	}

	// Fallback to file storage
	return c.getCredentialsFromFile()
}

// getCredentialsFromKeyring gets credentials from keyring.
func (c *Config) getCredentialsFromKeyring() (Credentials, error) {
	combined, err := keyring.Get(keyringServiceName, keyringUsername)
	if err != nil {
		return Credentials{}, err
	}
	return parseCredentials(combined)
}

// getCredentialsFromFile retrieves credentials from file
func (c *Config) getCredentialsFromFile() (Credentials, error) {
	credentialsFile := c.credentialsFileName()
	data, err := os.ReadFile(credentialsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return Credentials{}, ErrNotLoggedIn
		}
		return Credentials{}, fmt.Errorf("failed to read credentials file: %w", err)
	}

	credentials := string(data)
	if credentials == "" {
		return Credentials{}, ErrNotLoggedIn
	}

	return parseCredentials(credentials)
}

// parseCredentials parses the stored credentials from JSON format.
func parseCredentials(combined string) (Credentials, error) {
	var creds Credentials
	if err := json.Unmarshal([]byte(combined), &creds); err != nil {
		return Credentials{}, fmt.Errorf("failed to parse credentials: %w", err)
	}

	if creds.Token == nil && creds.APIKey == "" {
		return Credentials{}, errors.New("no valid credentials found")
	}
	if creds.ProjectID == "" {
		return Credentials{}, errors.New("project ID not found in stored credentials")
	}

	return creds, nil
}

// RemoveCredentials removes stored credentials from keyring and file fallback.
// When Keyring is disabled, the keyring is left untouched so that a logout in
// one config dir doesn't wipe credentials shared by another.
func (c *Config) RemoveCredentials() error {
	if c.Keyring {
		// Remove from keyring (ignore errors as it might not exist)
		c.removeCredentialsFromKeyring()
	}
	return c.removeCredentialsFile()
}

// removeCredentialsFromKeyring removes credentials from keyring
func (c *Config) removeCredentialsFromKeyring() {
	keyring.Delete(keyringServiceName, keyringUsername)
}

// removeCredentialsFile removes credentials file
func (c *Config) removeCredentialsFile() error {
	credentialsFile := c.credentialsFileName()
	if err := os.Remove(credentialsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove credentials file: %w", err)
	}
	return nil
}
